// Package protocol implements the PostgreSQL wire protocol.
package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"

	"github.com/adrianmcphee/smarterbase/internal/executor"
	"github.com/adrianmcphee/smarterbase/internal/storage"
	"github.com/jackc/pgproto3/v2"
)

// Server handles PostgreSQL wire protocol connections
type Server struct {
	listener net.Listener
	port     int
	executor *executor.Executor
}

// NewServer creates a new protocol server with storage
func NewServer(port int, dataDir string) (*Server, error) {
	store, err := storage.NewStore(dataDir)
	if err != nil {
		return nil, fmt.Errorf("create store: %w", err)
	}

	return &Server{
		port:     port,
		executor: executor.NewExecutor(store),
	}, nil
}

// Start begins listening for connections
func (s *Server) Start() error {
	var err error
	s.listener, err = net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", s.port, err)
	}

	log.Printf("SmarterBase listening on port %d", s.port)
	log.Printf("Connect with: psql -h localhost -p %d", s.port)

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			log.Printf("accept error: %v", err)
			continue
		}
		go s.handleConnection(conn)
	}
}

// handleConnection processes a single client connection
func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	log.Printf("New connection from %s", conn.RemoteAddr())

	// Handle SSL negotiation first (before creating backend)
	if err := s.handleSSLRequest(conn); err != nil {
		log.Printf("SSL handling error: %v", err)
		return
	}

	// Create backend (server-side) protocol handler
	backend := pgproto3.NewBackend(pgproto3.NewChunkReader(conn), conn)

	// Main message loop
	for {
		msg, err := backend.Receive()
		if err != nil {
			if err == io.EOF {
				log.Printf("Client disconnected")
			} else {
				log.Printf("receive error: %v", err)
			}
			return
		}

		switch m := msg.(type) {
		case *pgproto3.Query:
			s.handleQuery(conn, m.String)

		case *pgproto3.Terminate:
			log.Printf("Client terminated connection")
			return

		default:
			log.Printf("Unhandled message type: %T", msg)
		}
	}
}

// handleSSLRequest checks for and declines SSL requests
func (s *Server) handleSSLRequest(conn net.Conn) error {
	// Read the first message length
	header := make([]byte, 4)
	_, err := io.ReadFull(conn, header)
	if err != nil {
		return fmt.Errorf("read header: %w", err)
	}

	msgLen := int(binary.BigEndian.Uint32(header))

	// Read the rest of the message
	msg := make([]byte, msgLen-4)
	_, err = io.ReadFull(conn, msg)
	if err != nil {
		return fmt.Errorf("read message: %w", err)
	}

	// Check if it's an SSL request (magic number 80877103)
	if msgLen == 8 {
		code := binary.BigEndian.Uint32(msg)
		if code == 80877103 {
			// SSL request - decline with 'N'
			log.Printf("SSL request received, declining")
			_, err = conn.Write([]byte{'N'})
			if err != nil {
				return fmt.Errorf("write SSL response: %w", err)
			}
			// Client will send regular startup next, recurse
			return s.handleSSLRequest(conn)
		}
	}

	// Not SSL - it's a startup message
	return s.processStartupMessage(conn, msg)
}

// processStartupMessage handles the startup after we've read it during SSL check
func (s *Server) processStartupMessage(conn net.Conn, msg []byte) error {
	// Parse protocol version
	if len(msg) < 4 {
		return fmt.Errorf("startup message too short")
	}

	protocolVersion := binary.BigEndian.Uint32(msg[0:4])
	majorVersion := protocolVersion >> 16
	minorVersion := protocolVersion & 0xFFFF

	log.Printf("Protocol version: %d.%d", majorVersion, minorVersion)

	// Parse parameters (null-terminated key-value pairs)
	params := make(map[string]string)
	data := msg[4:]
	for len(data) > 1 {
		// Find key
		keyEnd := 0
		for keyEnd < len(data) && data[keyEnd] != 0 {
			keyEnd++
		}
		if keyEnd >= len(data) {
			break
		}
		key := string(data[:keyEnd])
		data = data[keyEnd+1:]

		// Find value
		valEnd := 0
		for valEnd < len(data) && data[valEnd] != 0 {
			valEnd++
		}
		if valEnd > len(data) {
			break
		}
		value := string(data[:valEnd])
		data = data[valEnd+1:]

		if key != "" {
			params[key] = value
		}
	}

	log.Printf("Startup: database=%s user=%s", params["database"], params["user"])

	// Build response manually
	buf := make([]byte, 0, 256)

	// AuthenticationOk: 'R' + int32(8) + int32(0)
	buf = append(buf, 'R')
	buf = appendInt32(buf, 8)
	buf = appendInt32(buf, 0)

	// ParameterStatus messages
	buf = appendParameterStatus(buf, "server_version", "15.0 (SmarterBase)")
	buf = appendParameterStatus(buf, "client_encoding", "UTF8")
	buf = appendParameterStatus(buf, "DateStyle", "ISO, MDY")
	buf = appendParameterStatus(buf, "server_encoding", "UTF8")
	buf = appendParameterStatus(buf, "TimeZone", "UTC")
	buf = appendParameterStatus(buf, "integer_datetimes", "on")

	// BackendKeyData: 'K' + int32(12) + int32(pid) + int32(secret)
	buf = append(buf, 'K')
	buf = appendInt32(buf, 12)
	buf = appendInt32(buf, 1234)
	buf = appendInt32(buf, 5678)

	// ReadyForQuery: 'Z' + int32(5) + byte('I')
	buf = append(buf, 'Z')
	buf = appendInt32(buf, 5)
	buf = append(buf, 'I')

	_, err := conn.Write(buf)
	return err
}

// handleQuery processes a simple query
func (s *Server) handleQuery(conn net.Conn, query string) {
	log.Printf("Query: %s", query)

	buf := make([]byte, 0, 512)

	// Handle special queries that clients expect
	switch {
	case query == "SELECT version()" || query == "SELECT version();":
		buf = appendRowDescription(buf, []string{"version"}, []int32{25})
		buf = appendDataRow(buf, []string{"SmarterBase 0.1.0 - PostgreSQL compatible file store"})
		buf = appendCommandComplete(buf, "SELECT 1")

	default:
		// Execute via SQL executor
		result, err := s.executor.Execute(query)
		if err != nil {
			buf = appendError(buf, err.Error())
		} else {
			// Send result based on type
			if len(result.Columns) > 0 {
				// SELECT query with results
				oids := make([]int32, len(result.Columns))
				for i := range oids {
					oids[i] = 25 // text OID
				}
				buf = appendRowDescription(buf, result.Columns, oids)
				for _, row := range result.Rows {
					buf = appendDataRow(buf, row)
				}
			}
			buf = appendCommandComplete(buf, result.Message)
		}
	}

	// ReadyForQuery
	buf = append(buf, 'Z')
	buf = appendInt32(buf, 5)
	buf = append(buf, 'I')

	conn.Write(buf)
}

// Helper functions to build protocol messages

func appendInt32(buf []byte, v int32) []byte {
	return append(buf, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

func appendInt16(buf []byte, v int16) []byte {
	return append(buf, byte(v>>8), byte(v))
}

func appendString(buf []byte, s string) []byte {
	buf = append(buf, s...)
	return append(buf, 0)
}

func appendParameterStatus(buf []byte, name, value string) []byte {
	// 'S' + int32(len) + name\0 + value\0
	msgLen := 4 + len(name) + 1 + len(value) + 1
	buf = append(buf, 'S')
	buf = appendInt32(buf, int32(msgLen))
	buf = appendString(buf, name)
	buf = appendString(buf, value)
	return buf
}

func appendRowDescription(buf []byte, names []string, oids []int32) []byte {
	// Calculate message length
	msgLen := 4 + 2 // length + field count
	for _, name := range names {
		msgLen += len(name) + 1 + 18 // name\0 + fixed fields
	}

	buf = append(buf, 'T')
	buf = appendInt32(buf, int32(msgLen))
	buf = appendInt16(buf, int16(len(names)))

	for i, name := range names {
		buf = appendString(buf, name)
		buf = appendInt32(buf, 0)       // table OID
		buf = appendInt16(buf, 0)       // column attr number
		buf = appendInt32(buf, oids[i]) // data type OID
		buf = appendInt16(buf, -1)      // data type size
		buf = appendInt32(buf, -1)      // type modifier
		buf = appendInt16(buf, 0)       // format code (text)
	}
	return buf
}

func appendDataRow(buf []byte, values []string) []byte {
	// Calculate message length
	msgLen := 4 + 2 // length + column count
	for _, v := range values {
		msgLen += 4 + len(v) // int32 length + value
	}

	buf = append(buf, 'D')
	buf = appendInt32(buf, int32(msgLen))
	buf = appendInt16(buf, int16(len(values)))

	for _, v := range values {
		buf = appendInt32(buf, int32(len(v)))
		buf = append(buf, v...)
	}
	return buf
}

func appendCommandComplete(buf []byte, tag string) []byte {
	msgLen := 4 + len(tag) + 1
	buf = append(buf, 'C')
	buf = appendInt32(buf, int32(msgLen))
	buf = appendString(buf, tag)
	return buf
}

func appendNotice(buf []byte, message string) []byte {
	// 'N' + length + 'M' + message\0 + \0
	msgLen := 4 + 1 + len(message) + 1 + 1
	buf = append(buf, 'N')
	buf = appendInt32(buf, int32(msgLen))
	buf = append(buf, 'M')
	buf = appendString(buf, message)
	buf = append(buf, 0) // terminator
	return buf
}

func appendError(buf []byte, message string) []byte {
	// 'E' + length + 'S' + "ERROR\0" + 'M' + message\0 + \0
	severity := "ERROR"
	msgLen := 4 + 1 + len(severity) + 1 + 1 + len(message) + 1 + 1
	buf = append(buf, 'E')
	buf = appendInt32(buf, int32(msgLen))
	buf = append(buf, 'S')
	buf = appendString(buf, severity)
	buf = append(buf, 'M')
	buf = appendString(buf, message)
	buf = append(buf, 0) // terminator
	return buf
}

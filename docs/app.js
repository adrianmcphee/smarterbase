const topbar = document.getElementById("layout-topbar")
if (topbar) {
    const handleScroll = () =>
        topbar.setAttribute("data-at-top", window.scrollY < 30 ? "true" : "false")

    window.addEventListener("scroll", handleScroll, { passive: true })
    handleScroll()
}

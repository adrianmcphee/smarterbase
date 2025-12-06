# SmarterBase Documentation Site

This directory contains the static site for SmarterBase, served via GitHub Pages.

## Local Development

```bash
cd docs
python3 -m http.server 8000
```

Then visit http://localhost:8000

## GitHub Pages Setup

This site is configured to be served from the `docs/` folder on the main branch.

To enable GitHub Pages:
1. Go to your repository Settings
2. Navigate to Pages
3. Under "Source", select "Deploy from a branch"
4. Select branch: `main` and folder: `/docs`
5. Save

The site is available at:
- https://smarterbase.com
- https://adrianmcphee.github.io/smarterbase/

## Files

- `index.html` - Main landing page
- `rfc/` - Technical specifications
- `adr/` - Architecture decision records

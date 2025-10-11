# SmarterBase Documentation Site

This directory contains the static site for SmarterBase documentation, served via GitHub Pages.

## Local Development

To test the site locally, you can use any static file server:

```bash
# Using Python
cd docs
python3 -m http.server 8000

# Using Node.js
npx serve docs

# Using Go
cd docs && go run -m http.server
```

Then visit http://localhost:8000 in your browser.

## GitHub Pages Setup

This site is configured to be served from the `docs/` folder on the main branch.

To enable GitHub Pages:
1. Go to your repository Settings
2. Navigate to Pages
3. Under "Source", select "Deploy from a branch"
4. Select branch: `main` and folder: `/docs`
5. Save

The site will be available at: `https://adrianmcphee.github.io/smarterbase/`

## Files

- `index.html` - Main landing page
- `assets/index.css` - Compiled CSS styles
- `app.js` - JavaScript for theme toggle and interactions
- `.nojekyll` - Prevents Jekyll processing on GitHub Pages

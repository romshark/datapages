import esbuild from "esbuild";

// Writes the document page bundle to app/static/editor.js and the stylesheets
// it imports to app/static/editor.css. Writes the styles of every page to
// app/static/style.css.
esbuild.buildSync({
  entryPoints: { editor: "src/index.ts", style: "src/style.css" },
  bundle: true,
  minify: true,
  format: "iife",
  outdir: "../app/static",
  target: ["es2020"],
  logLevel: "info",
});

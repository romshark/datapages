import esbuild from "esbuild";

// Bundles the Web Components into app/static/components.js, which the page
// head loads. make bundle and datapages watch run this.
esbuild.buildSync({
  entryPoints: ["src/index.ts"],
  bundle: true,
  minify: true,
  format: "iife",
  outfile: "../app/static/components.js",
  target: ["es2020"],
  logLevel: "info",
});

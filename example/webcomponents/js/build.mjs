import esbuild from "esbuild";

esbuild.buildSync({
  entryPoints: ["src/index.ts"],
  bundle: true,
  minify: true,
  format: "iife",
  outfile: "../app/static/bundle.js",
  target: ["es2020"],
  logLevel: "info",
});

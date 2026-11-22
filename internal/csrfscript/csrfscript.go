// Package csrfscript holds the script that adds the CSRF token to the Datastar
// requests of a page. [github.com/romshark/datapages/runtime/auth] writes it and
// [github.com/romshark/datapages/datapagestest] reads the token out of it.
//
// The script overrides fetch because Datastar sends its requests through it.
// Global fetch interceptors stack, and overriding it reaches them all.
package csrfscript

const (
	// Open starts the script element, before an optional nonce attribute.
	Open = "\n\t<script type=\"module\""

	// Prefix ends the start tag and precedes the token, which goes into a
	// JavaScript string literal.
	Prefix = `>
		const o = globalThis.fetch.bind(globalThis)
		globalThis.fetch=(i,init={}) => {
			const isReq=i instanceof Request
			const r=isReq ? i:new Request(i,init)
			if (r.headers.get("Datastar-Request")!=="true" ||
				r.method=="GET"||r.method=="HEAD"||r.method=="OPTIONS"||r.method=="QUERY"||
				new URL(r.url,location.href).origin!==location.origin
			) return isReq ? o(r,init):o(r)
			const h=new Headers(r.headers)
			h.set("X-CSRF-Token",'`

	// Suffix follows the token.
	Suffix = `')
			return o(new Request(r,{...init,headers:h}))
		}
	</script>`
)

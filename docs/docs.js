// The menu, the current section of the contents and the search of the
// documentation pages. The pages load this file as a module, which runs it
// after the document is parsed.

const root = new URL(document.body.dataset.siteRoot || "./", location.href);

// The navigation is a column on a wide screen and an overlay on a narrow one.
// In the overlay a link closes it after the jump, which in the column it must
// not do. The data-neo-sidebar-close attribute of Morpheus would close it in
// both.
const nav = document.getElementById("docs-nav");
document.getElementById("docs-nav-toggle")?.addEventListener("click", () => nav.toggle());
nav?.addEventListener("click", (e) => {
	if (e.target.closest("a") && nav.matches(":state(overlay)")) {
		nav.hide();
	}
});

// openVersion opens href, the same page in another version of the site, or
// fallback, the first page of that version, if the version lacks the page,
// such as one added after the release.
const openVersion = async (href, fallback) => {
	let target = new URL(href, location.href).href;
	try {
		const r = await fetch(target, { method: "HEAD" });
		if (!r.ok) {
			target = new URL(fallback, location.href).href;
		}
	} catch {
		// Without an answer the page opens as linked.
	}
	location.href = target;
};

// The version menu of the header opens the page in the version picked.
const versions = document.querySelector("neo-select.version-select");
if (versions) {
	const current = versions.getAttribute("value");
	versions.addEventListener("neo-select-change", (e) => {
		const option = [...versions.querySelectorAll("neo-option")]
			.find((o) => o.getAttribute("value") === e.detail.value);
		if (option && e.detail.value !== current) {
			openVersion(option.dataset.href, option.dataset.fallback);
		}
	});
	// Back from the other version, the browser restores this page with the
	// version that was picked.
	addEventListener("pageshow", (e) => {
		if (e.persisted) {
			versions.value = current;
		}
	});
}

// The link of the note on a page of main. A click that opens a new tab or
// window follows the link as written.
for (const a of document.querySelectorAll("a[data-fallback]")) {
	a.addEventListener("click", (e) => {
		if (e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) {
			return;
		}
		e.preventDefault();
		openVersion(a.getAttribute("href"), a.dataset.fallback);
	});
}

// The contents column marks the section whose heading was scrolled past last.
const sections = [...document.querySelectorAll(".docs-toc a")]
	.map((a) => [a, document.getElementById(decodeURIComponent(a.hash.slice(1)))])
	.filter(([, heading]) => heading);
if (sections.length > 0) {
	let scheduled = false;
	const mark = () => {
		scheduled = false;
		let current = null;
		for (const [a, heading] of sections) {
			if (heading.getBoundingClientRect().top > 96) {
				break;
			}
			current = a;
		}
		for (const [a] of sections) {
			if (a === current) {
				a.setAttribute("aria-current", "true");
			} else {
				a.removeAttribute("aria-current");
			}
		}
	};
	addEventListener("scroll", () => {
		if (!scheduled) {
			scheduled = true;
			requestAnimationFrame(mark);
		}
	}, { passive: true });
	mark();
}

// The search lists the sections of search.json that contain every word of the
// query, ranked by where the words occur: in the heading, in the page title,
// or in the text.
const search = document.querySelector("neo-dialog.search");
if (search) {
	const trigger = search.querySelector("[data-neo-dialog-trigger]");
	const dialog = search.querySelector("dialog");
	const input = search.querySelector(".search-input");
	const list = search.querySelector(".search-results");
	const status = search.querySelector(".search-status");
	const limit = 20;

	let index = null;
	const load = () => {
		index ??= fetch(new URL("search.json", root))
			.then((r) => {
				if (!r.ok) {
					throw new Error(r.statusText);
				}
				return r.json();
			})
			.then((entries) => entries.map((e) => ({
				...e,
				head: (e.s || e.p).toLowerCase(),
				page: e.p.toLowerCase(),
				text: e.t.toLowerCase(),
			})))
			.catch((err) => {
				index = null;
				throw err;
			});
		return index;
	};

	const words = (q) => q.toLowerCase().split(/\s+/).filter(Boolean);

	const score = (e, ws) => {
		let s = 0;
		for (const w of ws) {
			if (e.head.includes(w)) {
				s += e.head.startsWith(w) ? 12 : 8;
			} else if (e.page.includes(w)) {
				s += 5;
			} else if (e.text.includes(w)) {
				s += 1;
			} else {
				return 0;
			}
		}
		if (ws.length > 1 && e.head.includes(ws.join(" "))) {
			s += 10;
		}
		return s;
	};

	const escapeRegExp = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

	// highlight appends s to el with every occurrence of the words in <mark>.
	const highlight = (el, s, ws) => {
		const re = new RegExp(ws.map(escapeRegExp).join("|"), "gi");
		let last = 0;
		for (const m of s.matchAll(re)) {
			el.append(s.slice(last, m.index));
			const mark = document.createElement("mark");
			mark.textContent = m[0];
			el.append(mark);
			last = m.index + m[0].length;
		}
		el.append(s.slice(last));
	};

	// excerpt returns about 160 characters of t around the first word found.
	const excerpt = (t, ws) => {
		const lower = t.toLowerCase();
		let at = -1;
		for (const w of ws) {
			const i = lower.indexOf(w);
			if (i >= 0 && (at < 0 || i < at)) {
				at = i;
			}
		}
		const start = Math.max(0, at - 40);
		const end = Math.min(t.length, start + 160);
		return (start > 0 ? "..." : "") + t.slice(start, end) + (end < t.length ? "..." : "");
	};

	const span = (className) => {
		const s = document.createElement("span");
		s.className = className;
		return s;
	};

	let results = [];
	let active = -1;

	const select = (i) => {
		active = i;
		for (const [j, li] of [...list.children].entries()) {
			li.setAttribute("aria-selected", String(j === i));
		}
		if (i < 0) {
			input.removeAttribute("aria-activedescendant");
			return;
		}
		input.setAttribute("aria-activedescendant", list.children[i].id);
		list.children[i].scrollIntoView({ block: "nearest" });
	};

	const go = (a) => {
		dialog.close();
		location.href = a.href;
	};

	const item = (e, i, ws) => {
		const li = document.createElement("li");
		li.id = "search-result-" + i;
		li.setAttribute("role", "option");
		const a = document.createElement("a");
		a.href = new URL(e.u, root).href;
		a.tabIndex = -1;
		if (e.s) {
			const page = span("result-page");
			page.textContent = e.p;
			a.append(page);
		}
		const head = span("result-section");
		highlight(head, e.s || e.p, ws);
		a.append(head);
		if (e.t) {
			const text = span("result-text");
			highlight(text, excerpt(e.t, ws), ws);
			a.append(text);
		}
		li.append(a);
		li.addEventListener("mousemove", () => {
			if (active !== i) {
				select(i);
			}
		});
		return li;
	};

	const render = async () => {
		const ws = words(input.value);
		if (ws.length === 0) {
			results = [];
			list.replaceChildren();
			status.textContent = "";
			input.setAttribute("aria-expanded", "false");
			select(-1);
			return;
		}
		let entries;
		try {
			entries = await load();
		} catch {
			status.textContent = "The search index did not load. Try again.";
			return;
		}
		// The query changed while the index loaded.
		if (words(input.value).join(" ") !== ws.join(" ")) {
			return;
		}
		results = entries
			.map((e) => [score(e, ws), e])
			.filter(([s]) => s > 0)
			.sort((a, b) => b[0] - a[0])
			.slice(0, limit)
			.map(([, e]) => e);
		list.replaceChildren(...results.map((e, i) => item(e, i, ws)));
		status.textContent = results.length === 0 ? "No results." : "";
		input.setAttribute("aria-expanded", String(results.length > 0));
		select(results.length > 0 ? 0 : -1);
	};

	input.addEventListener("input", render);
	input.addEventListener("keydown", (e) => {
		if (e.key === "ArrowDown" && results.length > 0) {
			e.preventDefault();
			select(Math.min(active + 1, results.length - 1));
		} else if (e.key === "ArrowUp" && results.length > 0) {
			e.preventDefault();
			select(Math.max(active - 1, 0));
		} else if (e.key === "Enter" && active >= 0) {
			e.preventDefault();
			go(list.children[active].querySelector("a"));
		} else if (e.key === "Escape") {
			// A search field takes the first Escape to clear itself, which
			// keeps the dialog from seeing it.
			e.preventDefault();
			dialog.close();
		}
	});
	list.addEventListener("click", (e) => {
		const a = e.target.closest("a");
		if (a) {
			e.preventDefault();
			go(a);
		}
	});

	// Loading the index when the pointer reaches the button saves the wait
	// for it after the first key.
	trigger.addEventListener("pointerenter", () => load().catch(() => {}), { once: true });
	trigger.addEventListener("click", () => {
		load().catch(() => {});
		requestAnimationFrame(() => input.select());
	});

	document.addEventListener("keydown", (e) => {
		if (dialog.open || e.altKey) {
			return;
		}
		const typing = e.target.closest?.("input, textarea, select, [contenteditable]");
		const slash = e.key === "/" && !e.metaKey && !e.ctrlKey && !typing;
		const k = (e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k";
		if (slash || k) {
			e.preventDefault();
			trigger.click();
		}
	});
}

// Mermaid diagrams. The renderer writes each mermaid code block as a
// <pre class="mermaid">, which Mermaid replaces with an SVG in the colors of
// the current theme, and draws again when the theme changes. A page without a
// diagram does not load Mermaid.
const diagrams = [...document.querySelectorAll("pre.mermaid")];
if (diagrams.length > 0) {
	const sources = diagrams.map((d) => d.textContent);
	const system = matchMedia("(prefers-color-scheme: dark)");
	const dark = () => {
		const theme = document.documentElement.dataset.theme;
		return theme ? theme === "dark" : system.matches;
	};
	import("https://cdn.jsdelivr.net/npm/mermaid@12.2.0/dist/mermaid.esm.min.mjs")
		.then(({ default: mermaid }) => {
			const draw = () => {
				mermaid.initialize({
					startOnLoad: false,
					theme: dark() ? "dark" : "neutral",
					// Mermaid 12 wraps a label line at 120 px, which splits lines such
					// as "events, templates". Mermaid 11 wrapped at 200 px, wide enough
					// for every label line in guide/.
					flowchart: { wrappingWidth: 200 },
				});
				diagrams.forEach((d, i) => {
					d.removeAttribute("data-processed");
					d.textContent = sources[i];
				});
				return mermaid.run({ nodes: diagrams });
			};
			draw();
			new MutationObserver(draw).observe(document.documentElement, {
				attributes: true,
				attributeFilter: ["data-theme"],
			});
			system.addEventListener("change", draw);
		})
		.catch((err) => console.error("loading Mermaid:", err));
}

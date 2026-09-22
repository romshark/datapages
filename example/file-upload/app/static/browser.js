// The client-side JavaScript of the Datapages file-upload example, all of it.
// It exists because Datastar cannot open, slice or stream a File. It keeps no
// application state and never changes the state of a file: the server decides
// what is uploading, what is paused and which byte comes next.
//
// A job is {id, chunk, offset, chunkSize}, where chunk is the URL of the chunk
// endpoint with its offset parameter left empty and chunkSize comes from the
// upload limit the server enforces.
(() => {
	const defaultChunkSize = 1 << 20;

	// 250ms doubling to a cap of 8s, which outlasts about half a minute of failures.
	// A longer outage is picked up by the online event instead.
	const retries = 8;

	// A browser opens only a handful of connections per origin and the page
	// stream holds one of them for as long as the tab is open. Sending every
	// picked file at once would leave none for the stream or for the actions.
	const maxParallel = 3;

	let picked = []; // Files of the last pick or drop, until the dialog is done.
	const transfers = new Map(); // upload id -> what this tab sends for it
	const waiting = [];
	let sending = 0;

	// The transfer this tab is taking over, and the file picked for it.
	let awaiting = "";
	let repicked = null;

	const sleep = (ms) => new Promise((done) => setTimeout(done, ms));
	const backoff = (attempt) => Math.min(8000, 250 * 2 ** (attempt - 1));

	function bind(job, file) {
		transfers.set(job.id, {
			chunk: job.chunk,
			offset: job.offset,
			chunkSize: job.chunkSize || defaultChunkSize,
			file: file,
			running: false,
			stopped: false,
		});
	}

	function schedule(id) {
		if (!waiting.includes(id)) waiting.push(id);
		pump();
	}

	function pump() {
		while (sending < maxParallel && waiting.length > 0) {
			const id = waiting.shift();
			const t = transfers.get(id);
			if (!t || t.running) continue;
			sending++;
			run(id).finally(() => {
				sending--;
				pump();
			});
		}
	}

	async function run(id) {
		const t = transfers.get(id);
		if (!t || t.running) return;
		t.running = true;
		t.stopped = false;
		let attempt = 0;
		try {
			while (!t.stopped && t.offset < t.file.size) {
				const end = Math.min(t.offset + t.chunkSize, t.file.size);
				let res;
				try {
					res = await fetch(t.chunk + t.offset, {
						method: "PUT",
						body: t.file.slice(t.offset, end),
					});
				} catch {
					if (++attempt > retries) return;
					await sleep(backoff(attempt));
					continue;
				}
				// 403 is paused or complete, 404 is gone and 409 is an offset
				// the store cannot use: the server ended this transfer.
				if (res.status === 404) {
					transfers.delete(id);
					return;
				}
				if (res.status === 403 || res.status === 409) return;
				if (!res.ok) {
					if (++attempt > retries) return;
					await sleep(backoff(attempt));
					continue;
				}
				attempt = 0;
				t.offset = end;
			}
		} finally {
			t.running = false;
		}
	}

	// A transfer whose attempts ran out while the browser was offline is
	// scheduled by nothing else, and only the browser knows the network is back.
	// Rescheduling a running one is a no-op.
	globalThis.addEventListener("online", () => {
		for (const id of transfers.keys()) schedule(id);
	});

	window.dpUpload = {
		pick(el) {
			picked = Array.from(el.files);
			// Cleared so that picking the same file again fires change.
			el.value = "";
		},

		drop(evt) {
			picked = evt.dataTransfer ? Array.from(evt.dataTransfer.files) : [];
		},

		// files describes the picked files for the upload dialog.
		files() {
			return picked.map((f) => ({
				name: f.name,
				size: f.size,
				type: f.type,
			}));
		},

		// start pairs the jobs with the picked files,
		// which are in the order this tab staged them.
		start(jobs) {
			jobs.forEach((job, i) => {
				if (!picked[i]) return;
				bind(job, picked[i]);
				schedule(job.id);
			});
			picked = [];
		},

		// go sends the bytes of one job, or does nothing when
		// this tab holds no File for it.
		go(job) {
			const t = transfers.get(job.id);
			if (t) {
				t.chunk = job.chunk;
				t.chunkSize = job.chunkSize || defaultChunkSize;
				// A running transfer knows where it stands.
				// Taking the offset of the job would repeat a chunk.
				if (t.running) return;
				t.offset = job.offset;
			} else if (repicked && awaiting === job.id) {
				bind(job, repicked);
				repicked = null;
				awaiting = "";
			} else {
				return;
			}
			schedule(job.id);
		},

		// claim opens the file dialog for a transfer this tab doesn't hold.
		// The dialog opens only from a click of the visitor, which this is.
		claim(id, repick) {
			awaiting = id;
			repick.click();
		},

		repick(el) {
			repicked = el.files[0] || null;
			el.value = "";
		},

		// reattachment describes the picked file, which the server checks
		// against the file it is missing bytes of.
		reattachment() {
			return {
				id: awaiting,
				name: repicked ? repicked.name : "",
				size: repicked ? repicked.size : 0,
			};
		},

		// resize applies a changed upload limit to the running transfers.
		// The chunk in flight keeps its size, the next one does not.
		resize(chunkSize) {
			for (const t of transfers.values()) {
				t.chunkSize = chunkSize || defaultChunkSize;
			}
		},

		// pause stops sending without waiting for the server to refuse the next chunk.
		pause(id) {
			const t = transfers.get(id);
			if (t) t.stopped = true;
		},

		forget(id) {
			const t = transfers.get(id);
			if (t) t.stopped = true;
			transfers.delete(id);
		},
	};
})();

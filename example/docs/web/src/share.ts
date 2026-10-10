// copyLink copies the page address for the Share button. navigator.clipboard
// exists only in a secure context, HTTPS or localhost. Elsewhere, such as
// plain HTTP on a LAN address, it copies a hidden text field with the copy command.
export async function copyLink(): Promise<void> {
  const url = location.href;
  if (navigator.clipboard) {
    return navigator.clipboard.writeText(url);
  }
  const field = document.createElement("textarea");
  field.value = url;
  field.readOnly = true;
  // A fixed, transparent field scrolls nothing into view when selected.
  field.style.position = "fixed";
  field.style.opacity = "0";
  document.body.append(field);
  field.select();
  field.setSelectionRange(0, url.length);
  const copied = document.execCommand("copy");
  field.remove();
  if (!copied) {
    throw new Error("the browser did not copy the link");
  }
}

declare global {
  interface Window {
    copyLink: typeof copyLink;
  }
}

window.copyLink = copyLink;

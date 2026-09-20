// Datapages offline: reflects the online/offline state in the DOM.
// Injected into every HTML page, and into a cached page by the service worker,
// which the middleware never sees. The offline-class placeholder is
// replaced at serve time.
(function () {
  'use strict';
  if (window.__datapagesNetState) return;
  window.__datapagesNetState = true;

  function apply(offline) {
    document.documentElement.classList.toggle(__OFFLINE_CLASS__, offline);
    var id = 'datapages-offline-banner';
    var el = document.getElementById(id);
    if (offline) {
      if (!el) {
        el = document.createElement('div');
        el.id = id;
        el.setAttribute('role', 'status');
        el.textContent = 'You are offline, showing saved content.';
        document.body.appendChild(el);
      }
    } else if (el) {
      el.remove();
    }
  }

  function refresh() { apply(!navigator.onLine); }
  window.addEventListener('online', refresh);
  window.addEventListener('offline', refresh);
  if (document.readyState !== 'loading') refresh();
  else document.addEventListener('DOMContentLoaded', refresh);
})();

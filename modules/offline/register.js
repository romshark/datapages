// Datapages offline: registers the service worker and reflects the
// online/offline state in the DOM. Injected into every page <head>.
// The script-url and offline-class placeholders below are replaced by the
// offline module at serve time.
(function () {
  'use strict';
  if ('serviceWorker' in navigator) {
    window.addEventListener('load', function () {
      navigator.serviceWorker.register('__SCRIPT_URL__').catch(function (err) {
        console.error('offline: service worker registration failed', err);
      });
    });
  }

  function apply(offline) {
    document.documentElement.classList.toggle(__OFFLINE_CLASS__, offline);
    var id = 'datapages-offline-banner';
    var el = document.getElementById(id);
    if (offline) {
      if (!el) {
        el = document.createElement('div');
        el.id = id;
        el.setAttribute('role', 'status');
        el.textContent = 'You are offline — showing saved content.';
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

// Datapages offline: registers the service worker.
// Injected only while the client reports no current worker.
// The script-url placeholder is replaced by the offline module at serve time.
(function () {
  'use strict';
  if (!('serviceWorker' in navigator)) return;
  window.addEventListener('load', function () {
    navigator.serviceWorker.register('__SCRIPT_URL__').catch(function (err) {
      console.error('offline: service worker registration failed', err);
    });
  });
})();

// The offline module replaces __SCRIPT_URL__ before serving this script.
(function () {
  'use strict';
  if (!('serviceWorker' in navigator)) return;
  window.addEventListener('load', function () {
    // Without a scope, the worker controls only the directory of its script URL.
    navigator.serviceWorker.register('__SCRIPT_URL__', { scope: '/' }).catch(function (err) {
      console.error('offline: service worker registration failed', err);
    });
  });
})();

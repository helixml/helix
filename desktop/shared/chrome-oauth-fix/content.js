// Fix Anthropic's server-side OAuth redirect_uri mangling.
//
// During the login→authorize redirect, Anthropic's server drops a slash from
// the redirect_uri: http://localhost:PORT/callback becomes http:/localhost:PORT/callback.
// This content script detects the mangled URL and reloads with the fix before
// the error page renders.
//
// Safe no-op when Anthropic fixes this — the regex won't match correct URLs.
//
// Bug: https://github.com/anthropics/claude-code/issues/36015
(function() {
  var url = window.location.href;

  if (url.indexOf('/oauth/authorize') === -1) return;
  if (url.indexOf('redirect_uri=') === -1) return;

  var fixed = url;

  // Fix decoded form: redirect_uri=http:/localhost → redirect_uri=http://localhost
  fixed = fixed.replace(/(redirect_uri=https?):\/([^\/])/g, '$1://$2');

  // Fix encoded form: redirect_uri=http%3A%2Flocalhost → redirect_uri=http%3A%2F%2Flocalhost
  fixed = fixed.replace(/(redirect_uri=https?)(%3[Aa]%2[Ff])(?!%2[Ff])/g, '$1$2%2F');

  if (fixed !== url) {
    window.location.replace(fixed);
  }
})();

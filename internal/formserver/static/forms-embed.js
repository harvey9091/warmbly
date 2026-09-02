/* Warmbly Forms embed. Renders auto-resizing iframes for every
 * <div data-warmbly-form="ID"> and wires popup triggers for every
 * element with data-warmbly-popup="ID". The form itself always runs on
 * the Warmbly origin, so the host page's CSS and scripts cannot break
 * it and no visitor data flows through the host page. */
(function () {
  "use strict";
  if (window.__warmblyFormsLoaded) return;
  window.__warmblyFormsLoaded = true;

  var script = document.currentScript;
  var base = "";
  if (script && script.src) {
    var a = document.createElement("a");
    a.href = script.src;
    base = a.protocol + "//" + a.host;
  }

  function frameUrl(id) {
    return base + "/f/" + encodeURIComponent(id) + "?embed=1";
  }

  function makeFrame(id) {
    var f = document.createElement("iframe");
    f.src = frameUrl(id);
    f.setAttribute("data-warmbly-frame", id);
    f.setAttribute("title", "Form");
    f.style.width = "100%";
    f.style.border = "0";
    f.style.display = "block";
    f.style.minHeight = "180px";
    f.style.background = "transparent";
    f.setAttribute("allowtransparency", "true");
    f.loading = "lazy";
    return f;
  }

  function mountInline() {
    var nodes = document.querySelectorAll("[data-warmbly-form]");
    for (var i = 0; i < nodes.length; i++) {
      var el = nodes[i];
      if (el.getAttribute("data-warmbly-mounted")) continue;
      el.setAttribute("data-warmbly-mounted", "1");
      el.appendChild(makeFrame(el.getAttribute("data-warmbly-form")));
    }
  }

  var overlay = null;
  function closePopup() {
    if (!overlay) return;
    document.removeEventListener("keydown", onKey, true);
    overlay.parentNode && overlay.parentNode.removeChild(overlay);
    overlay = null;
  }
  function onKey(e) {
    if (e.key === "Escape") closePopup();
  }

  function openPopup(id) {
    closePopup();
    overlay = document.createElement("div");
    overlay.style.cssText =
      "position:fixed;inset:0;z-index:2147483000;background:rgba(15,23,42,.55);" +
      "display:flex;align-items:center;justify-content:center;padding:20px;overflow:auto";
    var box = document.createElement("div");
    box.style.cssText =
      "position:relative;width:100%;max-width:640px;max-height:92vh;overflow:auto;" +
      "border-radius:12px;background:transparent";
    var close = document.createElement("button");
    close.type = "button";
    close.setAttribute("aria-label", "Close");
    close.innerHTML = "&#10005;";
    close.style.cssText =
      "position:absolute;top:6px;right:6px;z-index:1;width:30px;height:30px;border:none;" +
      "border-radius:15px;background:rgba(15,23,42,.55);color:#fff;font-size:14px;cursor:pointer";
    close.onclick = closePopup;
    var frame = makeFrame(id);
    frame.loading = "eager";
    box.appendChild(close);
    box.appendChild(frame);
    overlay.appendChild(box);
    overlay.addEventListener("mousedown", function (e) {
      if (e.target === overlay) closePopup();
    });
    document.addEventListener("keydown", onKey, true);
    document.body.appendChild(overlay);
  }

  function wirePopups() {
    var nodes = document.querySelectorAll("[data-warmbly-popup]");
    for (var i = 0; i < nodes.length; i++) {
      var el = nodes[i];
      if (el.getAttribute("data-warmbly-mounted")) continue;
      el.setAttribute("data-warmbly-mounted", "1");
      el.addEventListener("click", function (e) {
        e.preventDefault();
        openPopup(this.getAttribute("data-warmbly-popup"));
      });
    }
  }

  window.addEventListener("message", function (e) {
    if (base && e.origin !== base) return;
    var d = e.data;
    if (!d || typeof d !== "object") return;
    if (d.type === "warmbly:resize" && d.form) {
      var frames = document.querySelectorAll('iframe[data-warmbly-frame="' + d.form + '"]');
      for (var i = 0; i < frames.length; i++) {
        if (e.source && frames[i].contentWindow !== e.source) continue;
        frames[i].style.height = Math.max(120, d.height | 0) + "px";
      }
    }
    if (d.type === "warmbly:submitted") {
      if (overlay) setTimeout(closePopup, 2400);
      try {
        document.dispatchEvent(new CustomEvent("warmbly:submitted", { detail: { form: d.form } }));
      } catch (err) {}
    }
  });

  window.WarmblyForms = { open: openPopup, close: closePopup, scan: function () { mountInline(); wirePopups(); } };

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", function () { mountInline(); wirePopups(); });
  } else {
    mountInline();
    wirePopups();
  }
})();

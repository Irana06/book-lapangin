(function () {
  // Tema: otomatis (ikut setting perangkat), terang, atau gelap. Pilihan disimpan
  // di localStorage; script kecil di <head> sudah menerapkannya sebelum halaman digambar.
  var modes = ["auto", "light", "dark"];
  var labels = { auto: "Otomatis", light: "Terang", dark: "Gelap" };
  var root = document.documentElement;

  function currentMode() {
    var t = root.dataset.theme;
    return t === "light" || t === "dark" ? t : "auto";
  }

  function saveMode(mode) {
    try {
      if (mode === "auto") localStorage.removeItem("theme");
      else localStorage.setItem("theme", mode);
    } catch (e) {}
  }

  function renderToggles() {
    var mode = currentMode();
    document.querySelectorAll("[data-theme-toggle]").forEach(function (btn) {
      btn.hidden = false;
      btn.dataset.mode = mode;
      btn.setAttribute("aria-label", "Tema: " + labels[mode] + ". Klik untuk mengganti.");
      var label = btn.querySelector("[data-theme-label]");
      if (label) label.textContent = labels[mode];
    });
  }

  document.addEventListener("click", function (e) {
    var btn = e.target.closest("[data-theme-toggle]");
    if (!btn) return;
    var next = modes[(modes.indexOf(currentMode()) + 1) % modes.length];
    if (next === "auto") delete root.dataset.theme;
    else root.dataset.theme = next;
    saveMode(next);
    renderToggles();
  });

  renderToggles();

  // Tampilkan pesan yang jelas kalau request HTMX gagal, bukan diam saja.
  function showError(event, fallback) {
    var xhr = event.detail.xhr;
    var text = xhr && xhr.responseText && xhr.responseText.length < 200 ? xhr.responseText.trim() : fallback;
    var target = event.detail.target;

    // Tombol aksi di kartu booking: pakai alert supaya kartu tidak hilang.
    if (!target || target.classList.contains("booking-item")) {
      window.alert(text);
      return;
    }
    var p = document.createElement("p");
    p.className = "notice notice-error";
    p.setAttribute("role", "alert");
    p.textContent = text;
    // Picker diganti seluruhnya, jadi pesan error ditaruh di panel slot.
    var slot = target.querySelector(".slot-panel [aria-live]");
    (slot || target).replaceChildren(p);
  }

  document.addEventListener("htmx:responseError", function (e) {
    showError(e, "Gagal memuat data. Coba muat ulang halaman.");
  });
  document.addEventListener("htmx:sendError", function (e) {
    showError(e, "Koneksi terputus. Periksa internet lalu coba lagi.");
  });
})();

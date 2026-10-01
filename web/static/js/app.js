// Tampilkan pesan yang jelas kalau request HTMX gagal, bukan diam saja.
(function () {
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
    target.replaceChildren(p);
  }

  document.addEventListener("htmx:responseError", function (e) {
    showError(e, "Gagal memuat data. Coba muat ulang halaman.");
  });
  document.addEventListener("htmx:sendError", function (e) {
    showError(e, "Koneksi terputus. Periksa internet lalu coba lagi.");
  });
})();

# Video demo ATS (Remotion)

Video 60 detik (1920 x 1080, 30 fps, H.264 yuv420p) untuk presentasi calon klien. Isinya satu cerita lamaran
dari aplikasi ini: kandidat fiktif **Nadia Pratama** melamar **Staf Administrasi Operasional**, mengisi profil
dari CV, memeriksa jabatan dan periode kerja pada sumbernya di CV, lalu mengirim lamaran. Setelah itu recruiter
**Maya Sari** (unit Operasional Pabrik) memindahkan lamaran tersebut ke tahap **Ujian**.

Semua gambar adalah layar asli aplikasi yang direkam dari stack demo lokal. Datanya sintetis (organisasi fiktif
PT Arunika Daya Persada, email `example.com`). Tidak ada mockup atau animasi pembacaan palsu.

Hasil render: `video/ats-demo/out/ats-product-demo.mp4`, dengan poster `out/poster.png`.

## Perintah (PowerShell, dari root repository)

Langkah 1 dan 2 hanya diperlukan untuk merekam ulang footage. Render cukup memakai aset di `public/`.

```powershell
# 0. Sekali saja: dependensi video (versi Remotion dipin 4.0.538)
cd video\ats-demo; npm install; cd ..\..

# 1. Stack demo terpisah: membuat/mereset database ats_video_dev, mengisi seed, build frontend produksi,
#    lalu menjalankan API :8090, worker :8091, worker CV :8092, dan web :5174.
#    Stack dev biasa (:8080/:5173) beserta database-nya tidak disentuh.
#    Prasyarat: PostgreSQL, binari backend (ops\windows\setup.ps1), alat CV (Poppler, Tesseract), dan .env.
powershell -NoProfile -ExecutionPolicy Bypass -File video\ats-demo\capture\demo-stack.ps1 start

# 2. Rekam footage (Playwright, konteks browser terpisah untuk HR Admin, Nadia, dan recruiter)
cd video\ats-demo; npm run capture; cd ..\..
powershell -NoProfile -ExecutionPolicy Bypass -File video\ats-demo\capture\demo-stack.ps1 stop

# 3. Edit di Studio (http://localhost:3000/AtsProductDemo)
cd video\ats-demo; npm run studio

# 4. Periksa, render, dan verifikasi
npm run typecheck
npm run stills     # out/stills/full dan out/stills/preview: awal, tengah, dan akhir tiap shot
npm run render     # out/ats-product-demo.mp4
npm run verify     # codec, resolusi, fps, yuv420p, 1800 frame, 60 detik, tanpa layar hitam/polos
npm run poster     # out/poster.png (shot bukti CV)
```

Capture menolak berjalan bila data demo sudah berisi lowongan dari rekaman sebelumnya. Jalankan `stop` lalu `start`
supaya database direset dan lamaran tidak tercatat ganda.

## Struktur

| Lokasi | Isi |
| --- | --- |
| `src/timeline.ts` | Seluruh keputusan editorial: urutan shot, durasi (frame), framing, kursor, caption, teks penutup |
| `src/AtsProductDemo.tsx` | Composition `AtsProductDemo`: shot (hard cut), pita caption, penutup |
| `src/ShotView.tsx`, `src/Cursor.tsx` | Kamera berbasis kotak elemen dari manifest, satu reframing, satu kursor |
| `capture/capture.mjs` | Rekaman Playwright, viewport 1440 x 900, deviceScaleFactor 2 |
| `capture/cv-nadia-pratama.html` | Sumber CV sintetis (dirender ke PDF saat capture) |
| `capture/demo-stack.ps1` | Stack demo terisolasi |
| `public/footage/` | Still dan `manifest.json` (route, aktor, state, kotak elemen; tanpa kredensial). Tidak di-commit |
| `public/fonts/` | Plus Jakarta Sans lokal (font aplikasi, lisensi OFL) |

Framing di `timeline.ts` memakai nama kotak elemen (misalnya `"apply"`, `"save"`) atau persegi dalam px CSS viewport
capture. Karena itu footage yang direkam ulang tetap terbingkai tepat. Render dibatalkan dengan pesan yang jelas
bila manifest, still, atau kotak yang dirujuk tidak ada.

## Catatan

- Tanggal lahir tidak diusulkan dari CV karena termasuk data pribadi sensitif. Capture mengisinya manual sebelum
  lamaran dikirim (still `c11`), tetapi momen itu tidak dimasukkan ke video.
- Aplikasi tidak menampilkan nama produk selain nama portal tenant ("Arunika Karier") dan "Back office rekrutmen",
  sehingga teks penutup memakai keduanya. Ubah `CLOSING` di `src/timeline.ts` bila ingin memakai nama lain.
- Capture memakai Chromium penuh (`channel: "chromium"`), karena headless shell mengabaikan `--lang`. Dengan begitu
  kontrol bawaan browser tampil dalam bahasa Indonesia ("Pilih File", tanggal 14/03/2002).
- Sesi login sementara tersimpan di `capture/.auth/` (diabaikan Git).

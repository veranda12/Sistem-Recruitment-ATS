package notify

// WhatsAppTemplates: teks ringkas bawaan untuk kanal WhatsApp (tanpa subjek). Hanya kejadian lamaran yang dikirim
// lewat WhatsApp; tautan sekali pakai (verifikasi, atur ulang sandi) tetap hanya lewat email.
var WhatsAppTemplates = map[string]Template{
	"application_submitted": {Key: "application_submitted", Version: 1,
		Body: "Halo {{nama}}, lamaran Anda untuk posisi {{lowongan}} sudah kami terima. Pantau statusnya di {{tautan}}"},
	"stage_changed": {Key: "stage_changed", Version: 1,
		Body: "Halo {{nama}}, lamaran {{lowongan}} Anda kini di tahap {{tahap}}. Detail: {{tautan}}"},
	"exam_invited": {Key: "exam_invited", Version: 1,
		Body: "Halo {{nama}}, Anda diundang ujian untuk posisi {{lowongan}}. Jadwal dan tombol mulai ada di {{tautan}}"},
	"result_released": {Key: "result_released", Version: 1,
		Body: "Halo {{nama}}, hasil ujian {{lowongan}} sudah dirilis. Tahap saat ini: {{tahap}}. Detail: {{tautan}}"},
	"interview_scheduled": {Key: "interview_scheduled", Version: 1,
		Body: "Halo {{nama}}, interview {{lowongan}} dijadwalkan {{waktu}}. Detail: {{tautan}}"},
	"interview_rescheduled": {Key: "interview_rescheduled", Version: 1,
		Body: "Halo {{nama}}, jadwal interview {{lowongan}} berubah dari {{waktu_lama}} menjadi {{waktu}}. Detail: {{tautan}}"},
	"interview_cancelled": {Key: "interview_cancelled", Version: 1,
		Body: "Halo {{nama}}, interview {{lowongan}} pada {{waktu}} dibatalkan. Kami akan menghubungi Anda bila ada jadwal baru."},
	"decision_hired": {Key: "decision_hired", Version: 1,
		Body: "Halo {{nama}}, selamat, Anda diterima untuk posisi {{lowongan}}. Tim kami akan menghubungi Anda."},
	"decision_rejected": {Key: "decision_rejected", Version: 1,
		Body: "Halo {{nama}}, terima kasih atas minat Anda pada posisi {{lowongan}}. Saat ini kami belum dapat melanjutkan lamaran Anda."},
}

package auth

import (
	"slices"

	"github.com/google/uuid"
)

// Katalog permission. Seed menulis ke tabel permissions; service memeriksa via Principal.Can.
const (
	PermJobsRead            = "jobs.read"
	PermJobsManage          = "jobs.manage"
	PermJobsPublish         = "jobs.publish"
	PermRequisitionsRead    = "requisitions.read"
	PermRequisitionsManage  = "requisitions.manage"
	PermRequisitionsApprove = "requisitions.approve"
	PermApplicationsRead    = "applications.read"
	PermApplicationsMove    = "applications.transition"
	PermApplicationsAssign  = "applications.assign"
	PermApprovalsDecide     = "approvals.decide"
	PermIdentityReveal      = "identity.reveal"
	PermPIIExport           = "pii.export"
	PermWorkflowsRead       = "workflows.read"
	PermWorkflowsManage     = "workflows.manage"
	PermWorkflowsPublish    = "workflows.publish"
	PermExamsManage         = "exams.manage"
	PermExamsGrade          = "exams.grade"
	PermResultsRelease      = "results.release"
	PermUsersManage         = "users.manage"
	PermRolesManage         = "roles.manage"
	PermSettingsManage      = "settings.manage"
	PermAuditRead           = "audit.read"
	PermCredentialsManage   = "credentials.manage"
	PermReportsRead         = "reports.read"
	PermReportsExport       = "reports.export"
	PermCVReplay            = "cv_parses.replay"
)

var PermissionCatalog = map[string]string{
	PermJobsRead: "Melihat lowongan internal", PermJobsManage: "Membuat dan mengubah lowongan",
	PermJobsPublish: "Menerbitkan dan menutup lowongan", PermRequisitionsRead: "Melihat permintaan tenaga kerja",
	PermRequisitionsManage: "Membuat permintaan tenaga kerja", PermRequisitionsApprove: "Menyetujui permintaan tenaga kerja",
	PermApplicationsRead: "Melihat lamaran", PermApplicationsMove: "Memindahkan tahap lamaran",
	PermApplicationsAssign: "Menugaskan recruiter", PermApprovalsDecide: "Memberi persetujuan keputusan",
	PermIdentityReveal: "Membuka NIK tersamar", PermPIIExport: "Mengekspor data pribadi",
	PermWorkflowsRead: "Melihat workflow", PermWorkflowsManage: "Mengubah draft workflow",
	PermWorkflowsPublish: "Menerbitkan workflow", PermExamsManage: "Mengelola bank soal dan jadwal",
	PermExamsGrade: "Menilai esai", PermResultsRelease: "Merilis nilai",
	PermUsersManage: "Mengelola pengguna internal", PermRolesManage: "Mengelola peran",
	PermSettingsManage: "Mengubah pengaturan", PermAuditRead: "Membaca audit",
	PermCredentialsManage: "Mengelola kredensial integrasi", PermReportsRead: "Melihat laporan",
	PermReportsExport: "Mengekspor laporan", PermCVReplay: "Menjalankan ulang pembacaan CV yang gagal permanen",
}

// Grant adalah satu permission pada satu scope. Unit nil dan Job nil = seluruh organisasi.
type Grant struct {
	Perm string
	Unit *uuid.UUID
	Job  *uuid.UUID
	Role string
}

// Resource yang diperiksa. Job opsional; bila grant terbatas pada job, resource harus menyebut job tsb.
type Resource struct {
	Unit uuid.UUID
	Job  *uuid.UUID
}

// Can menerapkan deny-by-default: harus ada grant yang cocok dengan permission dan scope resource.
func (p *Principal) Can(perm string, r Resource) bool {
	if p == nil {
		return false
	}
	for _, g := range p.Grants {
		if g.Perm != perm {
			continue
		}
		if g.Job != nil {
			if r.Job != nil && *r.Job == *g.Job {
				return true
			}
			continue
		}
		if g.Unit == nil || *g.Unit == r.Unit {
			return true
		}
	}
	return false
}

// CanAny: punya permission pada scope apa pun (untuk menampilkan menu/daftar yang kemudian difilter di query).
func (p *Principal) CanAny(perm string) bool {
	if p == nil {
		return false
	}
	return slices.ContainsFunc(p.Grants, func(g Grant) bool { return g.Perm == perm })
}

// Scope untuk filter query daftar.
type Scope struct {
	OrgWide bool
	Units   []uuid.UUID
	Jobs    []uuid.UUID
}

func (p *Principal) Scope(perm string) Scope {
	s := Scope{Units: []uuid.UUID{}, Jobs: []uuid.UUID{}}
	if p == nil {
		return s
	}
	for _, g := range p.Grants {
		if g.Perm != perm {
			continue
		}
		switch {
		case g.Job != nil:
			s.Jobs = append(s.Jobs, *g.Job)
		case g.Unit != nil:
			s.Units = append(s.Units, *g.Unit)
		default:
			s.OrgWide = true
		}
	}
	return s
}

func (s Scope) Empty() bool { return !s.OrgWide && len(s.Units) == 0 && len(s.Jobs) == 0 }

// RolesFor: peran yang memberi permission perm pada resource r (dipakai guard approval).
func (p *Principal) RolesFor(perm string, r Resource) []string {
	var out []string
	for _, g := range p.Grants {
		if g.Perm != perm || slices.Contains(out, g.Role) {
			continue
		}
		single := Principal{Grants: []Grant{g}}
		if single.Can(perm, r) {
			out = append(out, g.Role)
		}
	}
	return out
}

func (p *Principal) Roles() []string {
	var out []string
	for _, g := range p.Grants {
		if !slices.Contains(out, g.Role) {
			out = append(out, g.Role)
		}
	}
	return out
}

func (p *Principal) Permissions() []string {
	var out []string
	for _, g := range p.Grants {
		if !slices.Contains(out, g.Perm) {
			out = append(out, g.Perm)
		}
	}
	slices.Sort(out)
	return out
}

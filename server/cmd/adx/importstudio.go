package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/caarlos0/env/v11"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/blobs"
)

// runImportStudio copies Creative Studio data out of the old server's data
// folder (its adx.db and files/) into this server: projects by name in the
// default workspace, then concepts, ads, versions, renders, decisions,
// comments and picks, with every file put in this server's file store. A
// project that already has creatives here is skipped, so running it twice
// adds nothing. Who did what isn't carried over: the old user ids mean
// nothing here.
//
//	adx import-studio <old-data-dir>
func runImportStudio(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: adx import-studio <old-data-dir>")
	}
	cfg, err := env.ParseAs[config]()
	if err != nil {
		return err
	}
	ctx := context.Background()
	old, err := sql.Open("sqlite", "file:"+filepath.Join(args[0], "adx.db")+"?_pragma=query_only(1)&_time_format=sqlite")
	if err != nil {
		return err
	}
	defer old.Close()
	oldFiles, err := blobs.New(filepath.Join(args[0], "files"))
	if err != nil {
		return err
	}
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return err
	}
	files, err := blobs.New(cfg.FilesDir)
	if err != nil {
		return err
	}
	imp := &studioImport{ctx: ctx, old: old, oldFiles: oldFiles, q: db.Writer(), files: files, fileIDs: map[int64]int64{}}
	n, err := imp.run()
	if err != nil {
		return err
	}
	slog.Info("import-studio done", "projects", n.projects, "files", n.files, "ads", n.ads, "versions", n.versions, "skipped", n.skipped)
	return nil
}

type studioImport struct {
	ctx      context.Context
	old      *sql.DB
	oldFiles *blobs.Store
	q        *store.Queries
	files    *blobs.Store
	fileIDs  map[int64]int64 // old file id -> new
}

type importCounts struct{ projects, files, ads, versions, skipped int }

func (s *studioImport) run() (importCounts, error) {
	var n importCounts
	if err := s.importFiles(&n); err != nil {
		return n, fmt.Errorf("files: %w", err)
	}
	rows, err := s.old.QueryContext(s.ctx, `SELECT c.project_id, p.name, c.prefix, c.kicker, c.page_name, c.review_key, c.review_active, c.brand_kit, c.created_at, c.updated_at
		FROM creative_projects c JOIN projects p ON p.id = c.project_id ORDER BY c.project_id`)
	if err != nil {
		return n, err
	}
	defer rows.Close()
	type cp struct {
		oldID                                     int64
		name, prefix, kicker, page, key, brandKit string
		active                                    int64
		created, updated                          time.Time
	}
	var list []cp
	for rows.Next() {
		var c cp
		if err := rows.Scan(&c.oldID, &c.name, &c.prefix, &c.kicker, &c.page, &c.key, &c.active, &c.brandKit, &c.created, &c.updated); err != nil {
			return n, err
		}
		list = append(list, c)
	}
	if err := rows.Err(); err != nil {
		return n, err
	}
	for _, c := range list {
		p, err := s.q.GetProjectByName(s.ctx, store.GetProjectByNameParams{WorkspaceID: core.DefaultWorkspace, Name: c.name})
		if errors.Is(err, sql.ErrNoRows) {
			p, err = s.q.CreateProject(s.ctx, store.CreateProjectParams{WorkspaceID: core.DefaultWorkspace, Name: c.name, CreatedAt: c.created})
		}
		if err != nil {
			return n, fmt.Errorf("project %q: %w", c.name, err)
		}
		if _, err := s.q.GetCreativeProject(s.ctx, p.ID); err == nil {
			slog.Info("import-studio: project already has creatives, skipped", "project", c.name)
			n.skipped++
			continue
		}
		if err := s.q.InsertCreativeProject(s.ctx, store.InsertCreativeProjectParams{
			ProjectID: p.ID, Prefix: c.prefix, Kicker: c.kicker, PageName: c.page, ReviewKey: c.key, CreatedAt: c.created, UpdatedAt: c.updated,
		}); err != nil {
			return n, fmt.Errorf("project %q: %w", c.name, err)
		}
		if err := s.q.UpdateCreativeSettings(s.ctx, store.UpdateCreativeSettingsParams{Prefix: c.prefix, Kicker: c.kicker, PageName: c.page, ReviewActive: c.active, UpdatedAt: c.updated, ProjectID: p.ID}); err != nil {
			return n, err
		}
		if err := s.q.SaveBrandKit(s.ctx, store.SaveBrandKitParams{BrandKit: c.brandKit, UpdatedAt: c.updated, ProjectID: p.ID}); err != nil {
			return n, err
		}
		if err := s.importProject(c.oldID, p.ID, &n); err != nil {
			return n, fmt.Errorf("project %q: %w", c.name, err)
		}
		n.projects++
	}
	return n, nil
}

func (s *studioImport) importFiles(n *importCounts) error {
	rows, err := s.old.QueryContext(s.ctx, `SELECT id, sha256, content_type, bytes, width, height, created_at FROM files ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, size int64
		var sum, ct string
		var w, h sql.NullInt64
		var at time.Time
		if err := rows.Scan(&id, &sum, &ct, &size, &w, &h, &at); err != nil {
			return err
		}
		if row, err := s.q.GetFileBySHA(s.ctx, sum); err == nil {
			s.fileIDs[id] = row.ID
			continue
		}
		f, err := s.oldFiles.Open(sum)
		if err != nil {
			return fmt.Errorf("file %s: %w", sum[:12], err)
		}
		_, _, err = s.files.Put(f, size+1)
		f.Close()
		if err != nil {
			return fmt.Errorf("file %s: %w", sum[:12], err)
		}
		row, err := s.q.InsertFile(s.ctx, store.InsertFileParams{Sha256: sum, ContentType: ct, Bytes: size, Width: w, Height: h, CreatedAt: at})
		if err != nil {
			return err
		}
		s.fileIDs[id] = row.ID
		n.files++
	}
	return rows.Err()
}

func (s *studioImport) importProject(oldID, newID int64, n *importCounts) error {
	// Renders keep their codes; ads refer to them by code.
	rrows, err := s.old.QueryContext(s.ctx, `SELECT code, pos, label, hidden, file_id, preview_file_id, created_at FROM creative_renders WHERE project_id = ? ORDER BY pos`, oldID)
	if err != nil {
		return err
	}
	for rrows.Next() {
		var code, label string
		var pos, hidden, fileID, previewID int64
		var at time.Time
		if err := rrows.Scan(&code, &pos, &label, &hidden, &fileID, &previewID, &at); err != nil {
			rrows.Close()
			return err
		}
		r, err := s.q.InsertRender(s.ctx, store.InsertRenderParams{ProjectID: newID, Code: code, Pos: pos, Label: label, FileID: s.fileIDs[fileID], PreviewFileID: s.fileIDs[previewID], CreatedAt: at})
		if err != nil {
			rrows.Close()
			return err
		}
		if hidden == 1 {
			if err := s.q.UpdateRender(s.ctx, store.UpdateRenderParams{Label: label, Hidden: 1, ID: r.ID}); err != nil {
				rrows.Close()
				return err
			}
		}
	}
	rrows.Close()

	conceptIDs := map[int64]int64{}
	crows, err := s.old.QueryContext(s.ctx, `SELECT id, code, pos, name, why, primary_text, headline, description, cta, copy, created_at, updated_at FROM creative_concepts WHERE project_id = ? ORDER BY pos`, oldID)
	if err != nil {
		return err
	}
	for crows.Next() {
		var id, pos int64
		var code, name, why, primary, headline, desc, cta, cp string
		var created, updated time.Time
		if err := crows.Scan(&id, &code, &pos, &name, &why, &primary, &headline, &desc, &cta, &cp, &created, &updated); err != nil {
			crows.Close()
			return err
		}
		k, err := s.q.InsertConcept(s.ctx, store.InsertConceptParams{ProjectID: newID, Code: code, Pos: pos, Name: name, CreatedAt: created, UpdatedAt: updated})
		if err != nil {
			crows.Close()
			return err
		}
		if err := s.q.UpdateConcept(s.ctx, store.UpdateConceptParams{Name: name, Why: why, PrimaryText: primary, Headline: headline, Description: desc, Cta: cta, Copy: cp, UpdatedAt: updated, ID: k.ID}); err != nil {
			crows.Close()
			return err
		}
		conceptIDs[id] = k.ID
	}
	crows.Close()

	adIDs := map[int64]int64{}
	arows, err := s.old.QueryContext(s.ctx, `SELECT id, concept_id, code, file_name, pos, fmt, lang, has_cta, render_code, draft, created_at, updated_at FROM creative_ads WHERE project_id = ? ORDER BY pos`, oldID)
	if err != nil {
		return err
	}
	for arows.Next() {
		var id, conceptID, pos, hasCTA int64
		var code, fileName, format, lang, render string
		var draft sql.NullString
		var created, updated time.Time
		if err := arows.Scan(&id, &conceptID, &code, &fileName, &pos, &format, &lang, &hasCTA, &render, &draft, &created, &updated); err != nil {
			arows.Close()
			return err
		}
		a, err := s.q.InsertAd(s.ctx, store.InsertAdParams{ProjectID: newID, ConceptID: conceptIDs[conceptID], Code: code, FileName: fileName, Pos: pos, Fmt: format, Lang: lang, HasCta: hasCTA, RenderCode: render, Draft: draft, Kind: "static", CreatedAt: created, UpdatedAt: updated})
		if err != nil {
			arows.Close()
			return err
		}
		adIDs[id] = a.ID
		n.ads++
	}
	arows.Close()

	versionIDs := map[int64]int64{}
	vrows, err := s.old.QueryContext(s.ctx, `SELECT v.id, v.ad_id, v.version, v.file_id, v.preview_file_id, v.design, v.source, v.created_at
		FROM creative_ad_versions v JOIN creative_ads a ON a.id = v.ad_id WHERE a.project_id = ? ORDER BY v.ad_id, v.version`, oldID)
	if err != nil {
		return err
	}
	for vrows.Next() {
		var id, adID, version, fileID, previewID int64
		var design sql.NullString
		var source string
		var at time.Time
		if err := vrows.Scan(&id, &adID, &version, &fileID, &previewID, &design, &source, &at); err != nil {
			vrows.Close()
			return err
		}
		v, err := s.q.InsertVersion(s.ctx, store.InsertVersionParams{AdID: adIDs[adID], Version: version, FileID: s.fileIDs[fileID], PreviewFileID: s.fileIDs[previewID], Design: design, Source: source, CreatedAt: at})
		if err != nil {
			vrows.Close()
			return err
		}
		versionIDs[id] = v.ID
		n.versions++
	}
	vrows.Close()

	drows, err := s.old.QueryContext(s.ctx, `SELECT d.version_id, d.name, d.status, d.created_at FROM creative_decisions d
		JOIN creative_ad_versions v ON v.id = d.version_id JOIN creative_ads a ON a.id = v.ad_id WHERE a.project_id = ? ORDER BY d.id`, oldID)
	if err != nil {
		return err
	}
	for drows.Next() {
		var versionID int64
		var name, status string
		var at time.Time
		if err := drows.Scan(&versionID, &name, &status, &at); err != nil {
			drows.Close()
			return err
		}
		if err := s.q.InsertDecision(s.ctx, store.InsertDecisionParams{VersionID: versionIDs[versionID], Name: name, Status: status, CreatedAt: at}); err != nil {
			drows.Close()
			return err
		}
	}
	drows.Close()

	mrows, err := s.old.QueryContext(s.ctx, `SELECT c.ad_id, c.version_id, c.name, c.body, c.pin_x, c.pin_y, c.resolved_at, c.created_at FROM creative_comments c
		JOIN creative_ads a ON a.id = c.ad_id WHERE a.project_id = ? ORDER BY c.id`, oldID)
	if err != nil {
		return err
	}
	for mrows.Next() {
		var adID, versionID int64
		var name, body string
		var x, y sql.NullFloat64
		var resolved sql.NullTime
		var at time.Time
		if err := mrows.Scan(&adID, &versionID, &name, &body, &x, &y, &resolved, &at); err != nil {
			mrows.Close()
			return err
		}
		c, err := s.q.InsertComment(s.ctx, store.InsertCommentParams{AdID: adIDs[adID], VersionID: versionIDs[versionID], Name: name, Body: body, PinX: x, PinY: y, CreatedAt: at})
		if err != nil {
			mrows.Close()
			return err
		}
		if resolved.Valid {
			if err := s.q.SetCommentResolved(s.ctx, store.SetCommentResolvedParams{ResolvedAt: resolved, ID: c.ID}); err != nil {
				mrows.Close()
				return err
			}
		}
	}
	mrows.Close()

	prows, err := s.old.QueryContext(s.ctx, `SELECT p.concept_id, p.render_code, p.name, p.created_at FROM creative_render_picks p
		JOIN creative_concepts k ON k.id = p.concept_id WHERE k.project_id = ? ORDER BY p.id`, oldID)
	if err != nil {
		return err
	}
	defer prows.Close()
	for prows.Next() {
		var conceptID int64
		var render, name string
		var at time.Time
		if err := prows.Scan(&conceptID, &render, &name, &at); err != nil {
			return err
		}
		if err := s.q.InsertPick(s.ctx, store.InsertPickParams{ConceptID: conceptIDs[conceptID], RenderCode: render, Name: name, CreatedAt: at}); err != nil {
			return err
		}
	}
	return prows.Err()
}

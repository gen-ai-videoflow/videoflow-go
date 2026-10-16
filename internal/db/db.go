package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"6eek-go/internal/models"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

func Open(databasePath string) (*DB, error) {
	conn, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// SQLite permits one writer; keep all operations on the connection whose
	// busy timeout and foreign-key pragmas are initialized below. New pooled
	// connections otherwise inherit neither setting and fail concurrent writes.
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)

	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = NORMAL;",
		"PRAGMA busy_timeout = 5000;",
		"PRAGMA foreign_keys = ON;",
	}
	for _, p := range pragmas {
		if _, err := conn.Exec(p); err != nil {
			return nil, fmt.Errorf("failed pragma %s: %w", p, err)
		}
	}

	d := &DB{conn: conn}
	if err := d.migrate(); err != nil {
		return nil, fmt.Errorf("migration failed: %w", err)
	}
	if err := d.ensureActiveProject(); err != nil {
		return nil, fmt.Errorf("ensure active project failed: %w", err)
	}
	return d, nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}

func (d *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS projects (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		is_active BOOLEAN DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS characters (
		id TEXT PRIMARY KEY,
		project_id TEXT,
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		appearance TEXT DEFAULT '',
		personality TEXT DEFAULT '',
		visual_rules_json TEXT DEFAULT '[]',
		voice_rules_json TEXT DEFAULT '[]',
		sample_dialogue TEXT DEFAULT '',
		reference_asset_ids_json TEXT DEFAULT '[]',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS assets (
		id TEXT PRIMARY KEY,
		project_id TEXT,
		type TEXT NOT NULL,
		name TEXT NOT NULL,
		file_path TEXT NOT NULL,
		tags_json TEXT DEFAULT '[]',
		description TEXT DEFAULT '',
		character_id TEXT,
		metadata_json TEXT DEFAULT '{}',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS scripts (
		id TEXT PRIMARY KEY,
		project_id TEXT,
		idea TEXT DEFAULT '',
		title TEXT DEFAULT '',
		summary TEXT DEFAULT '',
		draft_json TEXT DEFAULT '{}',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS scenes (
		id TEXT PRIMARY KEY,
		project_id TEXT,
		script_id TEXT,
		scene_order INTEGER DEFAULT 0,
		title TEXT DEFAULT '',
		summary TEXT DEFAULT '',
		duration INTEGER DEFAULT 5,
		aspect_ratio TEXT DEFAULT '16:9',
		character_ids_json TEXT DEFAULT '[]',
		asset_ids_json TEXT DEFAULT '[]',
		scene_json TEXT DEFAULT '{}',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS shots (
		id TEXT PRIMARY KEY,
		scene_id TEXT NOT NULL,
		shot_order INTEGER DEFAULT 0,
		duration INTEGER DEFAULT 5,
		prompt TEXT DEFAULT '',
		camera TEXT,
		movement TEXT,
		asset_ids_json TEXT DEFAULT '[]',
		shot_json TEXT DEFAULT '{}',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(scene_id) REFERENCES scenes(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS render_jobs (
		id TEXT PRIMARY KEY,
		project_id TEXT,
		scene_id TEXT,
		shot_id TEXT,
		provider TEXT NOT NULL,
		model TEXT NOT NULL,
		provider_job_id TEXT,
		status TEXT NOT NULL DEFAULT 'pending',
		stage TEXT,
		progress TEXT,
		request_json TEXT DEFAULT '{}',
		response_json TEXT DEFAULT '{}',
		error TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS render_outputs (
		id TEXT PRIMARY KEY,
		render_job_id TEXT NOT NULL,
		video_path TEXT NOT NULL,
		thumbnail_path TEXT,
		captioned_path TEXT,
		captions_json TEXT DEFAULT '[]',
		score REAL,
		qa_json TEXT DEFAULT '{}',
		selected BOOLEAN DEFAULT 0,
		notes TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(render_job_id) REFERENCES render_jobs(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS style_guides (
		id TEXT PRIMARY KEY,
		project_id TEXT,
		name TEXT DEFAULT 'Default',
		style_prompt TEXT DEFAULT '',
		palette TEXT DEFAULT '',
		lighting TEXT DEFAULT '',
		audience TEXT DEFAULT '',
		tone TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS ops (
		id TEXT PRIMARY KEY,
		kind TEXT NOT NULL,
		status TEXT DEFAULT 'running',
		scene_id TEXT,
		output_id TEXT,
		project_id TEXT,
		error TEXT,
		result_json TEXT DEFAULT '{}',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS app_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS agent_settings (
		agent TEXT NOT NULL,
		project_id TEXT,
		provider TEXT DEFAULT '',
		model TEXT DEFAULT '',
		prompt_template TEXT DEFAULT '',
		PRIMARY KEY (agent, project_id)
	);

	CREATE TABLE IF NOT EXISTS provider_secrets (
		provider TEXT PRIMARY KEY,
		api_key TEXT DEFAULT '',
		base_url TEXT,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS connections (
		name TEXT PRIMARY KEY,
		label TEXT NOT NULL,
		preset TEXT NOT NULL,
		protocol TEXT NOT NULL,
		base_url TEXT,
		model TEXT DEFAULT '',
		mode TEXT DEFAULT 'auto',
		vision BOOLEAN DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS revisions (
		id TEXT PRIMARY KEY,
		entity_type TEXT NOT NULL,
		entity_id TEXT NOT NULL,
		snapshot_json TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := d.conn.Exec(schema)
	return err
}

func (d *DB) ensureActiveProject() error {
	var count int
	err := d.conn.QueryRow("SELECT COUNT(*) FROM projects WHERE is_active = 1").Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		var total int
		if err := d.conn.QueryRow("SELECT COUNT(*) FROM projects").Scan(&total); err != nil {
			return err
		}
		if total == 0 {
			id := "project_" + uuid.New().String()[:8]
			now := time.Now().UTC()
			_, err = d.conn.Exec(
				"INSERT INTO projects (id, name, description, is_active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
				id, "Default Project", "Default workspace", 1, now, now,
			)
			if err != nil {
				return err
			}
			log.Printf("Created initial active project: %s", id)
		} else {
			_, err = d.conn.Exec("UPDATE projects SET is_active = 1 WHERE rowid = (SELECT rowid FROM projects LIMIT 1)")
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// --- Projects ---

func (d *DB) GetActiveProjectID() (string, error) {
	var id string
	err := d.conn.QueryRow("SELECT id FROM projects WHERE is_active = 1 LIMIT 1").Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (d *DB) GetActiveProject() (*models.Project, error) {
	var p models.Project
	var isAct int
	err := d.conn.QueryRow("SELECT id, name, description, is_active, created_at, updated_at FROM projects WHERE is_active = 1 LIMIT 1").
		Scan(&p.ID, &p.Name, &p.Description, &isAct, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	p.IsActive = isAct == 1
	return &p, nil
}

func (d *DB) ListProjects() ([]models.Project, error) {
	rows, err := d.conn.Query("SELECT id, name, description, is_active, created_at, updated_at FROM projects ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []models.Project
	for rows.Next() {
		var p models.Project
		var isAct int
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &isAct, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.IsActive = isAct == 1
		projects = append(projects, p)
	}
	return projects, nil
}

func (d *DB) GetProject(id string) (*models.Project, error) {
	var p models.Project
	var isAct int
	err := d.conn.QueryRow("SELECT id, name, description, is_active, created_at, updated_at FROM projects WHERE id = ?", id).
		Scan(&p.ID, &p.Name, &p.Description, &isAct, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	p.IsActive = isAct == 1
	return &p, nil
}

func (d *DB) CreateProject(name, description string) (*models.Project, error) {
	id := "project_" + uuid.New().String()[:8]
	now := time.Now().UTC()
	_, err := d.conn.Exec(
		"INSERT INTO projects (id, name, description, is_active, created_at, updated_at) VALUES (?, ?, ?, 0, ?, ?)",
		id, name, description, now, now,
	)
	if err != nil {
		return nil, err
	}
	return &models.Project{
		ID:          id,
		Name:        name,
		Description: description,
		IsActive:    false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (d *DB) UpdateProject(id, name, description string) error {
	now := time.Now().UTC()
	_, err := d.conn.Exec("UPDATE projects SET name = ?, description = ?, updated_at = ? WHERE id = ?", name, description, now, id)
	return err
}

func (d *DB) DeleteProject(id string) error {
	_, err := d.conn.Exec("DELETE FROM projects WHERE id = ?", id)
	return err
}

func (d *DB) ActivateProject(id string) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("UPDATE projects SET is_active = 0"); err != nil {
		return err
	}
	res, err := tx.Exec("UPDATE projects SET is_active = 1, updated_at = ? WHERE id = ?", time.Now().UTC(), id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil || rows == 0 {
		return fmt.Errorf("project not found: %s", id)
	}
	return tx.Commit()
}

// --- Characters ---

func (d *DB) ListCharacters(projectID string) ([]models.Character, error) {
	rows, err := d.conn.Query(`
		SELECT id, project_id, name, description, appearance, personality,
		       visual_rules_json, voice_rules_json, sample_dialogue, reference_asset_ids_json,
		       created_at, updated_at
		FROM characters
		WHERE project_id = ?
		ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chars []models.Character
	for rows.Next() {
		var c models.Character
		var vr, vor, ra string
		if err := rows.Scan(
			&c.ID, &c.ProjectID, &c.Name, &c.Description, &c.Appearance, &c.Personality,
			&vr, &vor, &c.SampleDialogue, &ra, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		c.VisualRulesJSON = []byte(vr)
		c.VoiceRulesJSON = []byte(vor)
		c.ReferenceAssetIDsJSON = []byte(ra)
		chars = append(chars, c)
	}
	return chars, nil
}

func (d *DB) GetCharacter(id string) (*models.Character, error) {
	var c models.Character
	var vr, vor, ra string
	err := d.conn.QueryRow(`
		SELECT id, project_id, name, description, appearance, personality,
		       visual_rules_json, voice_rules_json, sample_dialogue, reference_asset_ids_json,
		       created_at, updated_at
		FROM characters WHERE id = ?
	`, id).Scan(
		&c.ID, &c.ProjectID, &c.Name, &c.Description, &c.Appearance, &c.Personality,
		&vr, &vor, &c.SampleDialogue, &ra, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	c.VisualRulesJSON = []byte(vr)
	c.VoiceRulesJSON = []byte(vor)
	c.ReferenceAssetIDsJSON = []byte(ra)
	return &c, nil
}

func (d *DB) CreateCharacter(c *models.Character) error {
	if c.ID == "" {
		c.ID = "char_" + uuid.New().String()[:8]
	}
	now := time.Now().UTC()
	c.CreatedAt = now
	c.UpdatedAt = now

	vr := string(c.VisualRulesJSON)
	if vr == "" {
		vr = "[]"
	}
	vor := string(c.VoiceRulesJSON)
	if vor == "" {
		vor = "[]"
	}
	ra := string(c.ReferenceAssetIDsJSON)
	if ra == "" {
		ra = "[]"
	}

	_, err := d.conn.Exec(`
		INSERT INTO characters (id, project_id, name, description, appearance, personality, visual_rules_json, voice_rules_json, sample_dialogue, reference_asset_ids_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID, c.ProjectID, c.Name, c.Description, c.Appearance, c.Personality, vr, vor, c.SampleDialogue, ra, now, now)
	return err
}

func (d *DB) UpdateCharacter(c *models.Character) error {
	now := time.Now().UTC()
	c.UpdatedAt = now
	vr := string(c.VisualRulesJSON)
	if vr == "" {
		vr = "[]"
	}
	vor := string(c.VoiceRulesJSON)
	if vor == "" {
		vor = "[]"
	}
	ra := string(c.ReferenceAssetIDsJSON)
	if ra == "" {
		ra = "[]"
	}

	_, err := d.conn.Exec(`
		UPDATE characters
		SET name = ?, description = ?, appearance = ?, personality = ?, visual_rules_json = ?, voice_rules_json = ?, sample_dialogue = ?, reference_asset_ids_json = ?, updated_at = ?
		WHERE id = ?
	`, c.Name, c.Description, c.Appearance, c.Personality, vr, vor, c.SampleDialogue, ra, now, c.ID)
	return err
}

func (d *DB) DeleteCharacter(id string) error {
	_, err := d.conn.Exec("DELETE FROM characters WHERE id = ?", id)
	return err
}

// --- Scenes & Scripts ---

func (d *DB) ListScenes(projectID string) ([]models.Scene, error) {
	rows, err := d.conn.Query(`
		SELECT id, project_id, script_id, scene_order, title, summary, duration,
		       aspect_ratio, character_ids_json, asset_ids_json, scene_json,
		       created_at, updated_at
		FROM scenes
		WHERE project_id = ?
		ORDER BY scene_order ASC, created_at ASC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var scenes []models.Scene
	for rows.Next() {
		var s models.Scene
		var cids, aids, sjson string
		if err := rows.Scan(
			&s.ID, &s.ProjectID, &s.ScriptID, &s.SceneOrder, &s.Title, &s.Summary, &s.Duration,
			&s.AspectRatio, &cids, &aids, &sjson, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		s.CharacterIDsJSON = []byte(cids)
		s.AssetIDsJSON = []byte(aids)
		s.SceneJSON = []byte(sjson)
		scenes = append(scenes, s)
	}
	return scenes, nil
}

func (d *DB) GetScene(id string) (*models.Scene, error) {
	var s models.Scene
	var cids, aids, sjson string
	err := d.conn.QueryRow(`
		SELECT id, project_id, script_id, scene_order, title, summary, duration,
		       aspect_ratio, character_ids_json, asset_ids_json, scene_json,
		       created_at, updated_at
		FROM scenes WHERE id = ?
	`, id).Scan(
		&s.ID, &s.ProjectID, &s.ScriptID, &s.SceneOrder, &s.Title, &s.Summary, &s.Duration,
		&s.AspectRatio, &cids, &aids, &sjson, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	s.CharacterIDsJSON = []byte(cids)
	s.AssetIDsJSON = []byte(aids)
	s.SceneJSON = []byte(sjson)
	return &s, nil
}

func (d *DB) CreateScene(s *models.Scene) error {
	if s.ID == "" {
		s.ID = "scene_" + uuid.New().String()[:8]
	}
	now := time.Now().UTC()
	s.CreatedAt = now
	s.UpdatedAt = now

	cids := string(s.CharacterIDsJSON)
	if cids == "" {
		cids = "[]"
	}
	aids := string(s.AssetIDsJSON)
	if aids == "" {
		aids = "[]"
	}
	sjson := string(s.SceneJSON)
	if sjson == "" {
		sjson = "{}"
	}

	_, err := d.conn.Exec(`
		INSERT INTO scenes (id, project_id, script_id, scene_order, title, summary, duration, aspect_ratio, character_ids_json, asset_ids_json, scene_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, s.ID, s.ProjectID, s.ScriptID, s.SceneOrder, s.Title, s.Summary, s.Duration, s.AspectRatio, cids, aids, sjson, now, now)
	return err
}

func (d *DB) UpdateScene(s *models.Scene) error {
	now := time.Now().UTC()
	s.UpdatedAt = now
	cids := string(s.CharacterIDsJSON)
	if cids == "" {
		cids = "[]"
	}
	aids := string(s.AssetIDsJSON)
	if aids == "" {
		aids = "[]"
	}
	sjson := string(s.SceneJSON)
	if sjson == "" {
		sjson = "{}"
	}

	_, err := d.conn.Exec(`
		UPDATE scenes
		SET title = ?, summary = ?, duration = ?, aspect_ratio = ?, scene_order = ?, character_ids_json = ?, asset_ids_json = ?, scene_json = ?, updated_at = ?
		WHERE id = ?
	`, s.Title, s.Summary, s.Duration, s.AspectRatio, s.SceneOrder, cids, aids, sjson, now, s.ID)
	return err
}

func (d *DB) DeleteScene(id string) error {
	_, err := d.conn.Exec("DELETE FROM scenes WHERE id = ?", id)
	return err
}

// --- Shots ---

func (d *DB) ListShots(sceneID string) ([]models.Shot, error) {
	rows, err := d.conn.Query(`
		SELECT id, scene_id, shot_order, duration, prompt, camera, movement,
		       asset_ids_json, shot_json, created_at, updated_at
		FROM shots
		WHERE scene_id = ?
		ORDER BY shot_order ASC, created_at ASC
	`, sceneID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shots []models.Shot
	for rows.Next() {
		var s models.Shot
		var aids, sjson string
		if err := rows.Scan(
			&s.ID, &s.SceneID, &s.ShotOrder, &s.Duration, &s.Prompt, &s.Camera, &s.Movement,
			&aids, &sjson, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		s.AssetIDsJSON = []byte(aids)
		s.ShotJSON = []byte(sjson)
		shots = append(shots, s)
	}
	return shots, nil
}

func (d *DB) GetShot(id string) (*models.Shot, error) {
	var s models.Shot
	var aids, sjson string
	err := d.conn.QueryRow(`
		SELECT id, scene_id, shot_order, duration, prompt, camera, movement,
		       asset_ids_json, shot_json, created_at, updated_at
		FROM shots WHERE id = ?
	`, id).Scan(
		&s.ID, &s.SceneID, &s.ShotOrder, &s.Duration, &s.Prompt, &s.Camera, &s.Movement,
		&aids, &sjson, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	s.AssetIDsJSON = []byte(aids)
	s.ShotJSON = []byte(sjson)
	return &s, nil
}

func (d *DB) CreateShot(s *models.Shot) error {
	if s.ID == "" {
		s.ID = "shot_" + uuid.New().String()[:8]
	}
	now := time.Now().UTC()
	s.CreatedAt = now
	s.UpdatedAt = now

	aids := string(s.AssetIDsJSON)
	if aids == "" {
		aids = "[]"
	}
	sjson := string(s.ShotJSON)
	if sjson == "" {
		sjson = "{}"
	}

	_, err := d.conn.Exec(`
		INSERT INTO shots (id, scene_id, shot_order, duration, prompt, camera, movement, asset_ids_json, shot_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, s.ID, s.SceneID, s.ShotOrder, s.Duration, s.Prompt, s.Camera, s.Movement, aids, sjson, now, now)
	return err
}

func (d *DB) UpdateShot(s *models.Shot) error {
	now := time.Now().UTC()
	s.UpdatedAt = now
	aids := string(s.AssetIDsJSON)
	if aids == "" {
		aids = "[]"
	}
	sjson := string(s.ShotJSON)
	if sjson == "" {
		sjson = "{}"
	}

	_, err := d.conn.Exec(`
		UPDATE shots
		SET prompt = ?, duration = ?, camera = ?, movement = ?, shot_order = ?, asset_ids_json = ?, shot_json = ?, updated_at = ?
		WHERE id = ?
	`, s.Prompt, s.Duration, s.Camera, s.Movement, s.ShotOrder, aids, sjson, now, s.ID)
	return err
}

func (d *DB) DeleteShot(id string) error {
	_, err := d.conn.Exec("DELETE FROM shots WHERE id = ?", id)
	return err
}

// --- Assets ---

func (d *DB) ListAssets(projectID string) ([]models.Asset, error) {
	rows, err := d.conn.Query(`
		SELECT id, project_id, type, name, file_path, tags_json, description,
		       character_id, metadata_json, created_at, updated_at
		FROM assets
		WHERE project_id = ?
		ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assets []models.Asset
	for rows.Next() {
		var a models.Asset
		var tags, meta string
		if err := rows.Scan(
			&a.ID, &a.ProjectID, &a.Type, &a.Name, &a.FilePath, &tags, &a.Description,
			&a.CharacterID, &meta, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		a.TagsJSON = []byte(tags)
		a.MetadataJSON = []byte(meta)
		assets = append(assets, a)
	}
	return assets, nil
}

func (d *DB) GetAsset(id string) (*models.Asset, error) {
	var a models.Asset
	var tags, meta string
	err := d.conn.QueryRow(`
		SELECT id, project_id, type, name, file_path, tags_json, description,
		       character_id, metadata_json, created_at, updated_at
		FROM assets WHERE id = ?
	`, id).Scan(
		&a.ID, &a.ProjectID, &a.Type, &a.Name, &a.FilePath, &tags, &a.Description,
		&a.CharacterID, &meta, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	a.TagsJSON = []byte(tags)
	a.MetadataJSON = []byte(meta)
	return &a, nil
}

func (d *DB) CreateAsset(a *models.Asset) error {
	if a.ID == "" {
		a.ID = "asset_" + uuid.New().String()[:8]
	}
	now := time.Now().UTC()
	a.CreatedAt = now
	a.UpdatedAt = now

	tags := string(a.TagsJSON)
	if tags == "" {
		tags = "[]"
	}
	meta := string(a.MetadataJSON)
	if meta == "" {
		meta = "{}"
	}

	_, err := d.conn.Exec(`
		INSERT INTO assets (id, project_id, type, name, file_path, tags_json, description, character_id, metadata_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, a.ID, a.ProjectID, a.Type, a.Name, a.FilePath, tags, a.Description, a.CharacterID, meta, now, now)
	return err
}

func (d *DB) UpdateAsset(a *models.Asset) error {
	now := time.Now().UTC()
	a.UpdatedAt = now
	tags := string(a.TagsJSON)
	if tags == "" {
		tags = "[]"
	}
	meta := string(a.MetadataJSON)
	if meta == "" {
		meta = "{}"
	}
	_, err := d.conn.Exec(`
		UPDATE assets
		SET type = ?, name = ?, tags_json = ?, description = ?, character_id = ?, metadata_json = ?, updated_at = ?
		WHERE id = ?
	`, a.Type, a.Name, tags, a.Description, a.CharacterID, meta, now, a.ID)
	return err
}

func (d *DB) DeleteAsset(id string) error {
	_, err := d.conn.Exec("DELETE FROM assets WHERE id = ?", id)
	return err
}

// --- Scripts ---

func (d *DB) ListScripts(projectID string) ([]models.Script, error) {
	rows, err := d.conn.Query("SELECT id, project_id, idea, title, summary, draft_json, created_at, updated_at FROM scripts WHERE project_id = ? ORDER BY created_at DESC", projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var scripts []models.Script
	for rows.Next() {
		var s models.Script
		var draft string
		if err := rows.Scan(&s.ID, &s.ProjectID, &s.Idea, &s.Title, &s.Summary, &draft, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		s.DraftJSON = []byte(draft)
		scripts = append(scripts, s)
	}
	return scripts, nil
}

func (d *DB) CreateScript(s *models.Script) error {
	if s.ID == "" {
		s.ID = "script_" + uuid.New().String()[:8]
	}
	now := time.Now().UTC()
	s.CreatedAt = now
	s.UpdatedAt = now
	draft := string(s.DraftJSON)
	if draft == "" {
		draft = "{}"
	}
	_, err := d.conn.Exec(
		"INSERT INTO scripts (id, project_id, idea, title, summary, draft_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		s.ID, s.ProjectID, s.Idea, s.Title, s.Summary, draft, now, now,
	)
	return err
}

// --- Style Guides ---

func (d *DB) GetStyleGuide(projectID string) (*models.StyleGuide, error) {
	var s models.StyleGuide
	err := d.conn.QueryRow(`
		SELECT id, project_id, name, style_prompt, palette, lighting, audience, tone, created_at, updated_at
		FROM style_guides WHERE project_id = ? LIMIT 1
	`, projectID).Scan(&s.ID, &s.ProjectID, &s.Name, &s.StylePrompt, &s.Palette, &s.Lighting, &s.Audience, &s.Tone, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (d *DB) UpsertStyleGuide(s *models.StyleGuide) error {
	now := time.Now().UTC()
	if s.ID == "" {
		s.ID = "style_" + uuid.New().String()[:8]
		s.CreatedAt = now
	}
	s.UpdatedAt = now

	_, err := d.conn.Exec(`
		INSERT INTO style_guides (id, project_id, name, style_prompt, palette, lighting, audience, tone, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			style_prompt = excluded.style_prompt,
			palette = excluded.palette,
			lighting = excluded.lighting,
			audience = excluded.audience,
			tone = excluded.tone,
			updated_at = excluded.updated_at
	`, s.ID, s.ProjectID, s.Name, s.StylePrompt, s.Palette, s.Lighting, s.Audience, s.Tone, s.CreatedAt, s.UpdatedAt)
	return err
}

// --- Render Jobs & Outputs ---

func (d *DB) GetRenderJob(id string) (*models.RenderJob, error) {
	row := d.conn.QueryRow(`
		SELECT id, project_id, scene_id, shot_id, provider, model, provider_job_id,
		       status, stage, progress, request_json, response_json, error, created_at, updated_at
		FROM render_jobs
		WHERE id = ?
	`, id)
	var j models.RenderJob
	var req, resp string
	if err := row.Scan(
		&j.ID, &j.ProjectID, &j.SceneID, &j.ShotID, &j.Provider, &j.Model, &j.ProviderJobID,
		&j.Status, &j.Stage, &j.Progress, &req, &resp, &j.Error, &j.CreatedAt, &j.UpdatedAt,
	); err != nil {
		return nil, err
	}
	j.RequestJSON = []byte(req)
	j.ResponseJSON = []byte(resp)
	return &j, nil
}

func (d *DB) ListRenderJobs(projectID string) ([]models.RenderJob, error) {
	rows, err := d.conn.Query(`
		SELECT id, project_id, scene_id, shot_id, provider, model, provider_job_id,
		       status, stage, progress, request_json, response_json, error, created_at, updated_at
		FROM render_jobs
		WHERE project_id = ?
		ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []models.RenderJob
	for rows.Next() {
		var j models.RenderJob
		var req, resp string
		if err := rows.Scan(
			&j.ID, &j.ProjectID, &j.SceneID, &j.ShotID, &j.Provider, &j.Model, &j.ProviderJobID,
			&j.Status, &j.Stage, &j.Progress, &req, &resp, &j.Error, &j.CreatedAt, &j.UpdatedAt,
		); err != nil {
			return nil, err
		}
		j.RequestJSON = []byte(req)
		j.ResponseJSON = []byte(resp)
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func (d *DB) CreateRenderJob(j *models.RenderJob) error {
	if j.ID == "" {
		j.ID = "job_" + uuid.New().String()[:8]
	}
	now := time.Now().UTC()
	j.CreatedAt = now
	j.UpdatedAt = now

	req := string(j.RequestJSON)
	if req == "" {
		req = "{}"
	}
	resp := string(j.ResponseJSON)
	if resp == "" {
		resp = "{}"
	}

	_, err := d.conn.Exec(`
		INSERT INTO render_jobs (id, project_id, scene_id, shot_id, provider, model, provider_job_id, status, stage, progress, request_json, response_json, error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, j.ID, j.ProjectID, j.SceneID, j.ShotID, j.Provider, j.Model, j.ProviderJobID, j.Status, j.Stage, j.Progress, req, resp, j.Error, now, now)
	return err
}

func (d *DB) UpdateRenderJob(j *models.RenderJob) error {
	now := time.Now().UTC()
	j.UpdatedAt = now

	_, err := d.conn.Exec(`
		UPDATE render_jobs
		SET status = ?, stage = ?, progress = ?, provider_job_id = ?, response_json = ?, error = ?, model = ?, provider = ?, request_json = ?, updated_at = ?
		WHERE id = ?
	`, j.Status, j.Stage, j.Progress, j.ProviderJobID, string(j.ResponseJSON), j.Error, j.Model, j.Provider, string(j.RequestJSON), now, j.ID)
	return err
}

func (d *DB) ListPendingRenderJobs() ([]models.RenderJob, error) {
	rows, err := d.conn.Query(`
		SELECT id, project_id, scene_id, shot_id, provider, model, provider_job_id,
		       status, stage, progress, request_json, response_json, error, created_at, updated_at
		FROM render_jobs
		WHERE status IN ('pending', 'running')
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []models.RenderJob
	for rows.Next() {
		var j models.RenderJob
		var req, resp string
		if err := rows.Scan(
			&j.ID, &j.ProjectID, &j.SceneID, &j.ShotID, &j.Provider, &j.Model, &j.ProviderJobID,
			&j.Status, &j.Stage, &j.Progress, &req, &resp, &j.Error, &j.CreatedAt, &j.UpdatedAt,
		); err != nil {
			return nil, err
		}
		j.RequestJSON = []byte(req)
		j.ResponseJSON = []byte(resp)
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func (d *DB) ListOutputsForJob(jobID string) ([]models.RenderOutput, error) {
	rows, err := d.conn.Query(`
		SELECT id, render_job_id, video_path, thumbnail_path, captioned_path, captions_json, score, qa_json, selected, notes, created_at, updated_at
		FROM render_outputs
		WHERE render_job_id = ?
		ORDER BY created_at DESC
	`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var outputs []models.RenderOutput
	for rows.Next() {
		var o models.RenderOutput
		var caps, qa string
		var sel int
		if err := rows.Scan(&o.ID, &o.RenderJobID, &o.VideoPath, &o.ThumbnailPath, &o.CaptionedPath, &caps, &o.Score, &qa, &sel, &o.Notes, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		o.CaptionsJSON = []byte(caps)
		o.QAJSON = []byte(qa)
		o.Selected = sel == 1
		outputs = append(outputs, o)
	}
	return outputs, nil
}

// --- Ops ---

func (d *DB) CreateOp(op *models.Op) error {
	if op.ID == "" {
		op.ID = "op_" + uuid.New().String()[:8]
	}
	now := time.Now().UTC()
	op.CreatedAt = now
	op.UpdatedAt = now

	res := string(op.ResultJSON)
	if res == "" {
		res = "{}"
	}

	_, err := d.conn.Exec(`
		INSERT INTO ops (id, kind, status, scene_id, output_id, project_id, error, result_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, op.ID, op.Kind, op.Status, op.SceneID, op.OutputID, op.ProjectID, op.Error, res, now, now)
	return err
}

func (d *DB) GetOp(id string) (*models.Op, error) {
	var op models.Op
	var res string
	err := d.conn.QueryRow(`
		SELECT id, kind, status, scene_id, output_id, project_id, error, result_json, created_at, updated_at
		FROM ops WHERE id = ?
	`, id).Scan(&op.ID, &op.Kind, &op.Status, &op.SceneID, &op.OutputID, &op.ProjectID, &op.Error, &res, &op.CreatedAt, &op.UpdatedAt)
	if err != nil {
		return nil, err
	}
	op.ResultJSON = []byte(res)
	return &op, nil
}

func (d *DB) UpdateOp(id, status string, resultJSON json.RawMessage, errorMsg *string) error {
	now := time.Now().UTC()
	res := string(resultJSON)
	if res == "" {
		res = "{}"
	}
	_, err := d.conn.Exec("UPDATE ops SET status = ?, result_json = ?, error = ?, updated_at = ? WHERE id = ?", status, res, errorMsg, now, id)
	return err
}

func (d *DB) ListOps(projectID string, limit int) ([]models.Op, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := d.conn.Query(`
		SELECT id, kind, status, scene_id, output_id, project_id, error, result_json, created_at, updated_at
		FROM ops
		WHERE project_id = ? OR project_id IS NULL
		ORDER BY created_at DESC
		LIMIT ?
	`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ops []models.Op
	for rows.Next() {
		var op models.Op
		var res string
		if err := rows.Scan(&op.ID, &op.Kind, &op.Status, &op.SceneID, &op.OutputID, &op.ProjectID, &op.Error, &res, &op.CreatedAt, &op.UpdatedAt); err != nil {
			return nil, err
		}
		op.ResultJSON = []byte(res)
		ops = append(ops, op)
	}
	return ops, nil
}

func (d *DB) ListOpsFiltered(projectID, kind string, limit int) ([]models.Op, error) {
	if limit <= 0 {
		limit = 20
	}
	query := "SELECT id, kind, status, scene_id, output_id, project_id, error, result_json, created_at, updated_at FROM ops WHERE 1=1"
	args := []interface{}{}
	if projectID != "" {
		query += " AND (project_id = ? OR project_id IS NULL)"
		args = append(args, projectID)
	}
	if kind != "" {
		query += " AND kind = ?"
		args = append(args, kind)
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ops := []models.Op{}
	for rows.Next() {
		var op models.Op
		var res string
		if err := rows.Scan(&op.ID, &op.Kind, &op.Status, &op.SceneID, &op.OutputID, &op.ProjectID, &op.Error, &res, &op.CreatedAt, &op.UpdatedAt); err != nil {
			return nil, err
		}
		op.ResultJSON = []byte(res)
		ops = append(ops, op)
	}
	return ops, nil
}

// --- App Settings ---

func (d *DB) GetAppSetting(key string) (string, error) {
	var val string
	err := d.conn.QueryRow("SELECT value FROM app_settings WHERE key = ?", key).Scan(&val)
	if err != nil {
		return "", err
	}
	return val, nil
}

func (d *DB) SetAppSetting(key, val string) error {
	_, err := d.conn.Exec("INSERT INTO app_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", key, val)
	return err
}

// --- Graph Visualization ---

func (d *DB) GetGraphData(projectID string) (*models.GraphResponse, error) {
	nodes := []models.GraphNode{}
	edges := []models.GraphEdge{}

	// Characters
	chars, err := d.ListCharacters(projectID)
	if err == nil {
		for _, c := range chars {
			nodes = append(nodes, models.GraphNode{
				ID:    c.ID,
				Type:  "character",
				Label: c.Name,
			})
		}
	}

	// Assets
	assets, err := d.ListAssets(projectID)
	if err == nil {
		for _, a := range assets {
			nodes = append(nodes, models.GraphNode{
				ID:    a.ID,
				Type:  "asset",
				Label: a.Name,
				Data: map[string]interface{}{
					"file_path":  a.FilePath,
					"asset_type": a.Type,
				},
			})
			if a.CharacterID != nil && *a.CharacterID != "" {
				edges = append(edges, models.GraphEdge{
					Source: *a.CharacterID,
					Target: a.ID,
					Label:  "reference",
				})
			}
			if len(a.MetadataJSON) > 0 {
				var meta struct {
					SceneID *string `json:"scene_id"`
				}
				if err := json.Unmarshal(a.MetadataJSON, &meta); err == nil && meta.SceneID != nil && *meta.SceneID != "" {
					edges = append(edges, models.GraphEdge{
						Source: *meta.SceneID,
						Target: a.ID,
						Label:  a.Type,
					})
				}
			}
		}
	}

	// Scenes
	scenes, err := d.ListScenes(projectID)
	if err == nil {
		for _, s := range scenes {
			nodes = append(nodes, models.GraphNode{
				ID:    s.ID,
				Type:  "scene",
				Label: s.Title,
				Data: map[string]interface{}{
					"aspect_ratio": s.AspectRatio,
					"duration":     s.Duration,
					"summary":      s.Summary,
				},
			})

			// Add casts / uses edges
			var charIDs []string
			_ = json.Unmarshal(s.CharacterIDsJSON, &charIDs)
			for _, cid := range charIDs {
				edges = append(edges, models.GraphEdge{
					Source: s.ID,
					Target: cid,
					Label:  "casts",
				})
			}
			var assetIDs []string
			_ = json.Unmarshal(s.AssetIDsJSON, &assetIDs)
			for _, aid := range assetIDs {
				edges = append(edges, models.GraphEdge{
					Source: s.ID,
					Target: aid,
					Label:  "uses",
				})
			}

			// Shots for scene
			shots, _ := d.ListShots(s.ID)
			for _, sh := range shots {
				nodes = append(nodes, models.GraphNode{
					ID:    sh.ID,
					Type:  "shot",
					Label: fmt.Sprintf("#%d %s", sh.ShotOrder+1, sh.Prompt),
					Data: map[string]interface{}{
						"scene_id":   sh.SceneID,
						"shot_order": sh.ShotOrder,
						"duration":   sh.Duration,
						"prompt":     sh.Prompt,
						"camera":     sh.Camera,
						"movement":   sh.Movement,
					},
				})
				edges = append(edges, models.GraphEdge{
					Source: s.ID,
					Target: sh.ID,
					Label:  "shot",
				})

				var shotAssetIDs []string
				_ = json.Unmarshal(sh.AssetIDsJSON, &shotAssetIDs)
				for _, aid := range shotAssetIDs {
					edges = append(edges, models.GraphEdge{
						Source: sh.ID,
						Target: aid,
						Label:  "uses",
					})
				}
			}
		}
	}

	// RenderJobs & RenderOutputs
	jobs, err := d.ListRenderJobs(projectID)
	if err == nil {
		for _, j := range jobs {
			modelName := j.Model
			if idx := strings.LastIndex(modelName, "/"); idx >= 0 {
				modelName = modelName[idx+1:]
			}
			nodes = append(nodes, models.GraphNode{
				ID:    j.ID,
				Type:  "render_job",
				Label: fmt.Sprintf("%s (%s)", j.Status, modelName),
				Data: map[string]interface{}{
					"status": j.Status,
				},
			})
			parentID := ""
			if j.ShotID != nil && *j.ShotID != "" {
				parentID = *j.ShotID
			} else if j.SceneID != nil && *j.SceneID != "" {
				parentID = *j.SceneID
			}
			if parentID != "" {
				edges = append(edges, models.GraphEdge{
					Source: parentID,
					Target: j.ID,
					Label:  "render",
				})
			}

			// Outputs for this job
			outputs, _ := d.ListOutputsForJob(j.ID)
			for _, o := range outputs {
				qaIssues := []string{}
				if len(o.QAJSON) > 0 {
					var qaData struct {
						Issues   []string `json:"issues"`
						QAIssues []string `json:"qa_issues"`
					}
					if err := json.Unmarshal(o.QAJSON, &qaData); err == nil {
						if len(qaData.Issues) > 0 {
							qaIssues = qaData.Issues
						} else if len(qaData.QAIssues) > 0 {
							qaIssues = qaData.QAIssues
						}
					}
				}
				label := o.ID
				if o.VideoPath != "" {
					label = filepath.Base(o.VideoPath)
				}
				nodes = append(nodes, models.GraphNode{
					ID:    o.ID,
					Type:  "output",
					Label: label,
					Data: map[string]interface{}{
						"video_path":     o.VideoPath,
						"thumbnail_path": o.ThumbnailPath,
						"captioned_path": o.CaptionedPath,
						"score":          o.Score,
						"qa_issues":      qaIssues,
						"render_job_id":  o.RenderJobID,
					},
				})
				edges = append(edges, models.GraphEdge{
					Source: j.ID,
					Target: o.ID,
					Label:  "output",
				})
			}
		}
	}

	// Filter valid edges
	knownNodes := make(map[string]bool)
	for _, n := range nodes {
		knownNodes[n.ID] = true
	}
	validEdges := []models.GraphEdge{}
	for _, e := range edges {
		if knownNodes[e.Source] && knownNodes[e.Target] {
			validEdges = append(validEdges, e)
		}
	}

	return &models.GraphResponse{
		Nodes: nodes,
		Edges: validEdges,
	}, nil
}

// --- Provider Secrets & Agent Settings ---

func (d *DB) GetProviderSecret(provider string) (apiKey string, baseURL *string, fromDB bool, err error) {
	var key, url sql.NullString
	err = d.conn.QueryRow("SELECT api_key, base_url FROM provider_secrets WHERE provider = ?", provider).Scan(&key, &url)
	if err == sql.ErrNoRows {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, err
	}
	if url.Valid {
		b := url.String
		baseURL = &b
	}
	return key.String, baseURL, true, nil
}

func (d *DB) SetProviderSecret(provider, apiKey string, baseURL *string) error {
	now := time.Now().UTC()
	var urlVal interface{}
	if baseURL != nil && *baseURL != "" {
		urlVal = *baseURL
	}
	_, err := d.conn.Exec(`
		INSERT INTO provider_secrets (provider, api_key, base_url, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(provider) DO UPDATE SET
			api_key = CASE WHEN ? = '' AND excluded.api_key = '' THEN '' WHEN ? != '' THEN excluded.api_key ELSE provider_secrets.api_key END,
			base_url = CASE WHEN ? IS NOT NULL THEN excluded.base_url ELSE provider_secrets.base_url END,
			updated_at = excluded.updated_at
	`, provider, apiKey, urlVal, now, apiKey, apiKey, urlVal)
	return err
}

func (d *DB) ListProviderSecrets() (map[string]models.ProviderSecret, error) {
	rows, err := d.conn.Query("SELECT provider, api_key, base_url, updated_at FROM provider_secrets")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := make(map[string]models.ProviderSecret)
	for rows.Next() {
		var ps models.ProviderSecret
		var bUrl sql.NullString
		if err := rows.Scan(&ps.Provider, &ps.APIKey, &bUrl, &ps.UpdatedAt); err != nil {
			return nil, err
		}
		if bUrl.Valid {
			u := bUrl.String
			ps.BaseURL = &u
		}
		res[ps.Provider] = ps
	}
	return res, nil
}

func (d *DB) GetAgentSettings() (map[string]models.AgentSetting, error) {
	rows, err := d.conn.Query("SELECT agent, provider, model FROM agent_settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := make(map[string]models.AgentSetting)
	for rows.Next() {
		var a models.AgentSetting
		if err := rows.Scan(&a.Agent, &a.Provider, &a.Model); err != nil {
			return nil, err
		}
		res[a.Agent] = a
	}
	return res, nil
}

func (d *DB) SetAgentSetting(agent, provider, model string) error {
	// Global rows have NULL project_id, which SQLite does not conflict on.
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM agent_settings WHERE agent = ? AND project_id IS NULL", agent); err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO agent_settings (agent, provider, model) VALUES (?, ?, ?)", agent, provider, model); err != nil {
		return err
	}
	err = tx.Commit()
	return err
}

func (d *DB) ResetAgentSetting(agent string) error {
	_, err := d.conn.Exec("DELETE FROM agent_settings WHERE agent = ?", agent)
	return err
}

// --- Connections ---

func (d *DB) CreateConnection(c *models.Connection) error {
	now := time.Now().UTC()
	c.CreatedAt = now
	_, err := d.conn.Exec(`
		INSERT INTO connections (name, label, preset, protocol, base_url, model, mode, vision, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.Name, c.Label, c.Preset, c.Protocol, c.BaseURL, c.Model, c.Mode, c.Vision, c.CreatedAt)
	return err
}

func (d *DB) ListConnections() ([]models.Connection, error) {
	rows, err := d.conn.Query("SELECT name, label, preset, protocol, base_url, model, mode, vision, created_at FROM connections ORDER BY created_at ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Connection
	for rows.Next() {
		var c models.Connection
		var bUrl sql.NullString
		if err := rows.Scan(&c.Name, &c.Label, &c.Preset, &c.Protocol, &bUrl, &c.Model, &c.Mode, &c.Vision, &c.CreatedAt); err != nil {
			return nil, err
		}
		if bUrl.Valid {
			u := bUrl.String
			c.BaseURL = &u
		}
		list = append(list, c)
	}
	return list, nil
}

// --- Outputs ---

func (d *DB) CreateRenderOutput(o *models.RenderOutput) error {
	if o.ID == "" {
		o.ID = "output_" + uuid.New().String()[:8]
	}
	now := time.Now().UTC()
	o.CreatedAt = now
	o.UpdatedAt = now

	caps := string(o.CaptionsJSON)
	if caps == "" {
		caps = "[]"
	}
	qa := string(o.QAJSON)
	if qa == "" {
		qa = "{}"
	}
	sel := 0
	if o.Selected {
		sel = 1
	}

	_, err := d.conn.Exec(`
		INSERT INTO render_outputs (
			id, render_job_id, video_path, thumbnail_path, captioned_path,
			captions_json, score, qa_json, selected, notes, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, o.ID, o.RenderJobID, o.VideoPath, o.ThumbnailPath, o.CaptionedPath,
		caps, o.Score, qa, sel, o.Notes, now, now)
	return err
}

func (d *DB) GetRenderOutput(id string) (*models.RenderOutput, error) {
	row := d.conn.QueryRow(`
		SELECT id, render_job_id, video_path, thumbnail_path, captioned_path, captions_json, score, qa_json, selected, notes, created_at, updated_at
		FROM render_outputs
		WHERE id = ?
	`, id)

	var o models.RenderOutput
	var caps, qa string
	var sel int
	if err := row.Scan(&o.ID, &o.RenderJobID, &o.VideoPath, &o.ThumbnailPath, &o.CaptionedPath, &caps, &o.Score, &qa, &sel, &o.Notes, &o.CreatedAt, &o.UpdatedAt); err != nil {
		return nil, err
	}
	o.CaptionsJSON = []byte(caps)
	o.QAJSON = []byte(qa)
	o.Selected = sel == 1
	return &o, nil
}

func (d *DB) UpdateRenderOutput(o *models.RenderOutput) error {
	now := time.Now().UTC()
	o.UpdatedAt = now
	sel := 0
	if o.Selected {
		sel = 1
	}
	_, err := d.conn.Exec(`
		UPDATE render_outputs
		SET video_path = ?, thumbnail_path = ?, captioned_path = ?, captions_json = ?, score = ?, qa_json = ?, selected = ?, notes = ?, updated_at = ?
		WHERE id = ?
	`, o.VideoPath, o.ThumbnailPath, o.CaptionedPath, string(o.CaptionsJSON), o.Score, string(o.QAJSON), sel, o.Notes, now, o.ID)
	return err
}

func (d *DB) SelectRenderOutput(id string, selected bool) error {
	o, err := d.GetRenderOutput(id)
	if err != nil {
		return err
	}
	// Deselect others in the same render job
	if selected {
		_, _ = d.conn.Exec("UPDATE render_outputs SET selected = 0 WHERE render_job_id = ?", o.RenderJobID)
	}
	sel := 0
	if selected {
		sel = 1
	}
	_, err = d.conn.Exec("UPDATE render_outputs SET selected = ? WHERE id = ?", sel, id)
	return err
}

// --- Shots Reorder & Scene/Shot Associations ---

func (d *DB) ReorderShots(sceneID string, orderedIDs []string) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for i, id := range orderedIDs {
		_, err := tx.Exec("UPDATE shots SET shot_order = ? WHERE id = ? AND scene_id = ?", i, id, sceneID)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) AddSceneCast(sceneID, characterID string) error {
	scene, err := d.GetScene(sceneID)
	if err != nil {
		return err
	}
	var charIDs []string
	_ = json.Unmarshal(scene.CharacterIDsJSON, &charIDs)
	for _, cid := range charIDs {
		if cid == characterID {
			return nil
		}
	}
	charIDs = append(charIDs, characterID)
	bytes, _ := json.Marshal(charIDs)
	scene.CharacterIDsJSON = bytes
	return d.UpdateScene(scene)
}

func (d *DB) RemoveSceneCast(sceneID, characterID string) error {
	scene, err := d.GetScene(sceneID)
	if err != nil {
		return err
	}
	var charIDs []string
	_ = json.Unmarshal(scene.CharacterIDsJSON, &charIDs)
	filtered := make([]string, 0, len(charIDs))
	for _, cid := range charIDs {
		if cid != characterID {
			filtered = append(filtered, cid)
		}
	}
	bytes, _ := json.Marshal(filtered)
	scene.CharacterIDsJSON = bytes
	return d.UpdateScene(scene)
}

func (d *DB) AttachShotAsset(shotID, assetID string) error {
	shot, err := d.GetShot(shotID)
	if err != nil {
		return err
	}
	var assetIDs []string
	_ = json.Unmarshal(shot.AssetIDsJSON, &assetIDs)
	for _, aid := range assetIDs {
		if aid == assetID {
			return nil
		}
	}
	assetIDs = append(assetIDs, assetID)
	bytes, _ := json.Marshal(assetIDs)
	shot.AssetIDsJSON = bytes
	return d.UpdateShot(shot)
}

func (d *DB) DetachShotAsset(shotID, assetID string) error {
	shot, err := d.GetShot(shotID)
	if err != nil {
		return err
	}
	var assetIDs []string
	_ = json.Unmarshal(shot.AssetIDsJSON, &assetIDs)
	filtered := make([]string, 0, len(assetIDs))
	for _, aid := range assetIDs {
		if aid != assetID {
			filtered = append(filtered, aid)
		}
	}
	bytes, _ := json.Marshal(filtered)
	shot.AssetIDsJSON = bytes
	return d.UpdateShot(shot)
}

// --- Revisions ---

func (d *DB) CreateRevision(entityType, entityID string, snapshot []byte) (*models.Revision, error) {
	id := "rev_" + uuid.New().String()[:8]
	now := time.Now().UTC()
	rev := &models.Revision{
		ID:           id,
		EntityType:   entityType,
		EntityID:     entityID,
		SnapshotJSON: snapshot,
		CreatedAt:    now,
	}
	_, err := d.conn.Exec(`
		INSERT INTO revisions (id, entity_type, entity_id, snapshot_json, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, rev.ID, rev.EntityType, rev.EntityID, string(rev.SnapshotJSON), rev.CreatedAt)
	if err != nil {
		return nil, err
	}
	return rev, nil
}

func (d *DB) ListRevisions(entityType, entityID string) ([]models.Revision, error) {
	rows, err := d.conn.Query(`
		SELECT id, entity_type, entity_id, snapshot_json, created_at
		FROM revisions
		WHERE entity_type = ? AND entity_id = ?
		ORDER BY created_at DESC
		LIMIT 20
	`, entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Revision
	for rows.Next() {
		var r models.Revision
		var snap string
		if err := rows.Scan(&r.ID, &r.EntityType, &r.EntityID, &snap, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.SnapshotJSON = []byte(snap)
		list = append(list, r)
	}
	return list, nil
}

func (d *DB) GetRevision(id string) (*models.Revision, error) {
	row := d.conn.QueryRow("SELECT id, entity_type, entity_id, snapshot_json, created_at FROM revisions WHERE id = ?", id)
	var r models.Revision
	var snap string
	if err := row.Scan(&r.ID, &r.EntityType, &r.EntityID, &snap, &r.CreatedAt); err != nil {
		return nil, err
	}
	r.SnapshotJSON = []byte(snap)
	return &r, nil
}

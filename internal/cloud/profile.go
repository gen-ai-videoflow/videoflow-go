package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type ProfileLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}
type profilePatch struct {
	Name     *string        `json:"name"`
	Bio      *string        `json:"bio"`
	Location *string        `json:"location"`
	Avatar   *string        `json:"avatar_url"`
	Banner   *string        `json:"banner_url"`
	Website  *string        `json:"website"`
	Links    *[]ProfileLink `json:"social_links"`
	Featured *string        `json:"featured_video_id"`
}

func validProfileURL(raw string, image bool) bool {
	if raw == "" {
		return true
	}
	if len(raw) > 2048 || strings.TrimSpace(raw) != raw {
		return false
	}
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return false
	}
	if image {
		ext := strings.ToLower(path.Ext(u.Path))
		if ext == ".svg" || ext == ".svgz" {
			return false
		}
	}
	return true
}
func (p profilePatch) validate() bool {
	if p.Name != nil && (strings.TrimSpace(*p.Name) == "" || len(*p.Name) > 100) {
		return false
	}
	if p.Bio != nil && len(*p.Bio) > 2000 || p.Location != nil && len(*p.Location) > 120 {
		return false
	}
	for _, v := range []*string{p.Avatar, p.Banner} {
		if v != nil && !validProfileURL(*v, true) {
			return false
		}
	}
	if p.Website != nil && !validProfileURL(*p.Website, false) {
		return false
	}
	if p.Links != nil {
		if len(*p.Links) > 8 {
			return false
		}
		for _, l := range *p.Links {
			if strings.TrimSpace(l.Label) == "" || len(l.Label) > 40 || l.URL == "" || !validProfileURL(l.URL, false) || !validPresetSocialLink(l) {
				return false
			}
		}
	}
	return p.Featured == nil || len(*p.Featured) <= 100
}
func (s *Server) richChannelData(ctx context.Context, handle string) (map[string]any, error) {
	var id, name, description, avatar, banner, bio, location, website, featured string
	var links []byte
	var joined time.Time
	e := s.Store.DB.QueryRowContext(ctx, `SELECT id,name,description,avatar_url,banner_url,bio,location,website,social_links,CASE WHEN EXISTS(SELECT 1 FROM cloud_videos v WHERE v.id=c.featured_video_id AND v.channel_id=c.id AND v.status='published' AND v.visibility='public') THEN featured_video_id ELSE '' END,created_at FROM cloud_channels c WHERE handle=$1`, handle).Scan(&id, &name, &description, &avatar, &banner, &bio, &location, &website, &links, &featured, &joined)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if e != nil {
		return nil, e
	}
	social := []ProfileLink{}
	if e = json.Unmarshal(links, &social); e != nil {
		return nil, e
	}
	var followers, videos, views int64
	e = s.Store.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM cloud_follows WHERE channel_id=$1),count(*),coalesce(sum(views),0) FROM cloud_videos WHERE channel_id=$1 AND status='published' AND visibility='public'`, id).Scan(&followers, &videos, &views)
	return map[string]any{"id": id, "name": name, "handle": handle, "description": description, "avatar_url": avatar, "banner_url": banner, "bio": bio, "location": location, "website": website, "social_links": social, "featured_video_id": featured, "joined_at": joined, "followers": followers, "public_video_count": videos, "public_views": views}, e
}
func (s *Server) profileRoutes(r chi.Router) {
	r.Get("/profile", s.getProfile)
	r.Patch("/profile", s.patchProfile)
	r.Post("/profile/images", s.uploadProfileImage)
}
func (s *Server) getProfile(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	c, e := s.richChannelData(r.Context(), u.Handle)
	s.result(w, map[string]any{"user": u, "channel": c}, e)
}
func (s *Server) patchProfile(w http.ResponseWriter, r *http.Request) {
	var p profilePatch
	if !s.decode(w, r, &p) {
		return
	}
	if !s.validateProfilePatch(p) {
		s.fail(w, ErrValidation)
		return
	}
	u := currentUser(r)
	tx, e := s.Store.DB.BeginTx(r.Context(), nil)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer tx.Rollback()
	// Serialize profile updates with publication state changes before validating a pin.
	if _, e = tx.ExecContext(r.Context(), `SELECT id FROM cloud_channels WHERE id=$1 FOR UPDATE`, u.ChannelID); e != nil {
		s.fail(w, e)
		return
	}
	for _, imageURL := range []*string{p.Avatar, p.Banner} {
		if imageURL == nil || *imageURL == "" {
			continue
		}
		prefix := strings.TrimSuffix(s.Config.PublicOrigin, "/") + "/api/v1/profile-images/"
		if s.Config.PublicOrigin != "" && strings.HasPrefix(*imageURL, prefix) {
			id := strings.TrimPrefix(*imageURL, prefix)
			var exists bool
			if e = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM cloud_profile_images WHERE id=$1 AND channel_id=$2)`, id, u.ChannelID).Scan(&exists); e != nil {
				s.fail(w, e)
				return
			}
			if !exists {
				s.fail(w, ErrValidation)
				return
			}
		}
	}
	if p.Featured != nil && *p.Featured != "" {
		var ok bool
		e = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM cloud_videos WHERE id=$1 AND channel_id=$2 AND status='published' AND visibility='public')`, *p.Featured, u.ChannelID).Scan(&ok)
		if e != nil {
			s.fail(w, e)
			return
		}
		if !ok {
			s.fail(w, ErrValidation)
			return
		}
	}
	var links any
	if p.Links != nil {
		b, err := json.Marshal(*p.Links)
		if err != nil {
			s.fail(w, err)
			return
		}
		links = string(b)
	}
	_, e = tx.ExecContext(r.Context(), `UPDATE cloud_channels SET name=coalesce($2,name),bio=coalesce($3,bio),description=coalesce($3,description),location=coalesce($4,location),avatar_url=coalesce($5,avatar_url),banner_url=coalesce($6,banner_url),website=coalesce($7,website),social_links=coalesce($8::jsonb,social_links),featured_video_id=coalesce($9,featured_video_id) WHERE id=$1`, u.ChannelID, p.Name, p.Bio, p.Location, p.Avatar, p.Banner, p.Website, links, p.Featured)
	if e == nil && p.Name != nil {
		_, e = tx.ExecContext(r.Context(), `UPDATE cloud_users SET name=$2 WHERE id=$1`, u.ID, *p.Name)
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		s.fail(w, e)
		return
	}
	if p.Name != nil {
		u.Name = *p.Name
	}
	c, e := s.richChannelData(r.Context(), u.Handle)
	s.result(w, map[string]any{"user": u, "channel": c}, e)
}
func (s *Store) DiscoveryFeed(ctx context.Context, u User, query, category, filter string) ([]Video, error) {
	if filter != "" && filter != "following" && filter != "all" {
		return nil, ErrValidation
	}
	if filter == "following" {
		category = "following"
	}
	return s.Discovery(ctx, u, query, category)
}

func validPresetSocialLink(link ProfileLink) bool {
	domains := map[string][]string{"youtube": {"youtube.com", "youtu.be"}, "instagram": {"instagram.com"}, "tiktok": {"tiktok.com"}, "linkedin": {"linkedin.com"}, "github": {"github.com"}, "x": {"x.com", "twitter.com"}, "facebook": {"facebook.com", "fb.com"}, "twitch": {"twitch.tv"}, "discord": {"discord.com", "discord.gg"}, "telegram": {"t.me", "telegram.me"}}
	allowed, known := domains[strings.ToLower(strings.TrimSpace(link.Label))]
	if !known {
		return true
	}
	u, err := url.Parse(link.URL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, domain := range allowed {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

package service

import (
	"context"
	"sort"
	"strings"
	"time"

	admincontrol "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
)

const settingsKeyAnnouncements = "admin_control.announcements"

func (s *Service) ListAnnouncements(ctx context.Context, opts admincontrol.ListOptions) (admincontrol.AnnouncementList, error) {
	var collection announcementCollection
	if err := s.loadTyped(ctx, settingsKeyAnnouncements, &collection); err != nil {
		return admincontrol.AnnouncementList{}, err
	}
	items := make([]admincontrol.Announcement, 0, len(collection.Items))
	for _, item := range collection.Items {
		if opts.Status != "" && string(item.Status) != opts.Status {
			continue
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	paged := pageItems(items, opts)
	return admincontrol.AnnouncementList{Items: paged, Total: len(items)}, nil
}

func (s *Service) CreateAnnouncement(ctx context.Context, req admincontrol.AnnouncementRequest, actorUserID int) (admincontrol.Announcement, error) {
	var collection announcementCollection
	if err := s.loadTyped(ctx, settingsKeyAnnouncements, &collection); err != nil {
		return admincontrol.Announcement{}, err
	}
	now := s.clock.Now()
	item, err := announcementFromCreateRequest(req, nextID(collection.NextID, len(collection.Items)), now)
	if err != nil {
		return admincontrol.Announcement{}, err
	}
	collection.Items = append(collection.Items, item)
	collection.NextID = item.ID + 1
	if err := s.saveTyped(ctx, settingsKeyAnnouncements, collection, actorUserID); err != nil {
		return admincontrol.Announcement{}, err
	}
	return item, nil
}

func (s *Service) UpdateAnnouncement(ctx context.Context, id int, req admincontrol.AnnouncementRequest, actorUserID int) (admincontrol.Announcement, error) {
	var collection announcementCollection
	if err := s.loadTyped(ctx, settingsKeyAnnouncements, &collection); err != nil {
		return admincontrol.Announcement{}, err
	}
	for idx, item := range collection.Items {
		if item.ID != id {
			continue
		}
		updated, err := announcementFromCreateRequest(req, id, s.clock.Now())
		if err != nil {
			return admincontrol.Announcement{}, err
		}
		updated.CreatedAt = item.CreatedAt
		collection.Items[idx] = updated
		if err := s.saveTyped(ctx, settingsKeyAnnouncements, collection, actorUserID); err != nil {
			return admincontrol.Announcement{}, err
		}
		return updated, nil
	}
	return admincontrol.Announcement{}, admincontrol.ErrNotFound
}

func (s *Service) DeleteAnnouncement(ctx context.Context, id int, actorUserID int) (admincontrol.Announcement, error) {
	var collection announcementCollection
	if err := s.loadTyped(ctx, settingsKeyAnnouncements, &collection); err != nil {
		return admincontrol.Announcement{}, err
	}
	for idx, item := range collection.Items {
		if item.ID != id {
			continue
		}
		collection.Items = append(collection.Items[:idx], collection.Items[idx+1:]...)
		if err := s.saveTyped(ctx, settingsKeyAnnouncements, collection, actorUserID); err != nil {
			return admincontrol.Announcement{}, err
		}
		return item, nil
	}
	return admincontrol.Announcement{}, admincontrol.ErrNotFound
}

func (s *Service) ListUserAnnouncements(ctx context.Context, user userscontract.User, opts admincontrol.ListOptions) (admincontrol.UserAnnouncementList, error) {
	if user.ID <= 0 {
		return admincontrol.UserAnnouncementList{}, admincontrol.ErrInvalidInput
	}
	var collection announcementCollection
	if err := s.loadTyped(ctx, settingsKeyAnnouncements, &collection); err != nil {
		return admincontrol.UserAnnouncementList{}, err
	}
	now := s.clock.Now()
	visible := make([]admincontrol.Announcement, 0, len(collection.Items))
	for _, item := range collection.Items {
		if !announcementVisibleToUser(item, user, now) {
			continue
		}
		visible = append(visible, item)
	}
	sort.SliceStable(visible, func(i, j int) bool { return visible[i].CreatedAt.After(visible[j].CreatedAt) })

	ids := announcementIDs(visible)
	reads, err := s.store.ListAnnouncementReads(ctx, user.ID, ids)
	if err != nil {
		return admincontrol.UserAnnouncementList{}, err
	}
	readByAnnouncement := announcementReadByID(reads)
	items := make([]admincontrol.UserAnnouncement, 0, len(visible))
	var unread int
	for _, item := range visible {
		userItem := admincontrol.UserAnnouncement{Announcement: item}
		if read, ok := readByAnnouncement[item.ID]; ok && !read.ReadAt.Before(item.UpdatedAt) {
			userItem.Read = true
			readAt := read.ReadAt
			userItem.ReadAt = &readAt
		} else {
			unread++
		}
		items = append(items, userItem)
	}
	return admincontrol.UserAnnouncementList{
		Items:  pageItems(items, opts),
		Total:  len(items),
		Unread: unread,
	}, nil
}

func (s *Service) MarkUserAnnouncementRead(ctx context.Context, user userscontract.User, announcementID int) (admincontrol.UserAnnouncement, error) {
	if user.ID <= 0 || announcementID <= 0 {
		return admincontrol.UserAnnouncement{}, admincontrol.ErrInvalidInput
	}
	var collection announcementCollection
	if err := s.loadTyped(ctx, settingsKeyAnnouncements, &collection); err != nil {
		return admincontrol.UserAnnouncement{}, err
	}
	now := s.clock.Now()
	for _, item := range collection.Items {
		if item.ID != announcementID {
			continue
		}
		if !announcementVisibleToUser(item, user, now) {
			return admincontrol.UserAnnouncement{}, admincontrol.ErrNotFound
		}
		reads, err := s.store.ListAnnouncementReads(ctx, user.ID, []int{announcementID})
		if err != nil {
			return admincontrol.UserAnnouncement{}, err
		}
		if len(reads) > 0 && !reads[0].ReadAt.Before(item.UpdatedAt) {
			readAt := reads[0].ReadAt
			return admincontrol.UserAnnouncement{
				Announcement: item,
				Read:         true,
				ReadAt:       &readAt,
			}, nil
		}
		read, err := s.store.MarkAnnouncementRead(ctx, user.ID, announcementID, now)
		if err != nil {
			return admincontrol.UserAnnouncement{}, err
		}
		readAt := read.ReadAt
		return admincontrol.UserAnnouncement{
			Announcement: item,
			Read:         true,
			ReadAt:       &readAt,
		}, nil
	}
	return admincontrol.UserAnnouncement{}, admincontrol.ErrNotFound
}

// AnnouncementReadStatus returns who has read one announcement (recent-first),
// for the admin read-status view.
func (s *Service) AnnouncementReadStatus(ctx context.Context, announcementID int) (admincontrol.AnnouncementReadStatus, error) {
	if announcementID <= 0 {
		return admincontrol.AnnouncementReadStatus{}, admincontrol.ErrInvalidInput
	}
	readers, err := s.store.ListAnnouncementReadsByAnnouncement(ctx, announcementID, 500)
	if err != nil {
		return admincontrol.AnnouncementReadStatus{}, err
	}
	return admincontrol.AnnouncementReadStatus{
		AnnouncementID: announcementID,
		Total:          len(readers),
		Readers:        readers,
	}, nil
}

func announcementVisibleToUser(item admincontrol.Announcement, user userscontract.User, now time.Time) bool {
	if item.Status != admincontrol.AnnouncementStatusPublished {
		return false
	}
	if item.StartsAt != nil && now.Before(item.StartsAt.UTC()) {
		return false
	}
	if item.EndsAt != nil && !now.Before(item.EndsAt.UTC()) {
		return false
	}
	if !announcementMatchesAudience(item.Audience, user.Roles) {
		return false
	}
	// Segments refine the audience: when present, at least one segment must
	// match the user. No segments = audience-only delivery (back-compat).
	if len(item.Segments) > 0 && !announcementMatchesSegments(item.Segments, user) {
		return false
	}
	return true
}

func announcementMatchesAudience(audience admincontrol.AnnouncementAudience, roles []userscontract.Role) bool {
	switch audience {
	case admincontrol.AnnouncementAudienceAll:
		return true
	case admincontrol.AnnouncementAudienceUsers:
		return !hasAdminRole(roles)
	case admincontrol.AnnouncementAudienceAdmins:
		return hasAdminRole(roles)
	default:
		return false
	}
}

func announcementMatchesSegments(segments []admincontrol.AnnouncementSegment, user userscontract.User) bool {
	for _, seg := range segments {
		if announcementSegmentMatches(seg, user) {
			return true
		}
	}
	return false
}

// announcementSegmentMatches is AND across the segment's non-empty conditions.
func announcementSegmentMatches(seg admincontrol.AnnouncementSegment, user userscontract.User) bool {
	if len(seg.Roles) > 0 && !userRolesIntersect(user.Roles, seg.Roles) {
		return false
	}
	if len(seg.UserIDs) > 0 && !containsInt(seg.UserIDs, user.ID) {
		return false
	}
	if len(seg.EmailDomains) > 0 && !emailDomainIn(user.Email, seg.EmailDomains) {
		return false
	}
	return true
}

func userRolesIntersect(roles []userscontract.Role, want []string) bool {
	for _, role := range roles {
		for _, w := range want {
			if strings.EqualFold(string(role), strings.TrimSpace(w)) {
				return true
			}
		}
	}
	return false
}

func containsInt(values []int, target int) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

func emailDomainIn(email string, domains []string) bool {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return false
	}
	domain := strings.ToLower(strings.TrimSpace(email[at+1:]))
	for _, d := range domains {
		if strings.ToLower(strings.TrimSpace(d)) == domain {
			return true
		}
	}
	return false
}

func hasAdminRole(roles []userscontract.Role) bool {
	for _, role := range roles {
		if role == userscontract.RoleOwner || role == userscontract.RoleAdmin {
			return true
		}
	}
	return false
}

func announcementIDs(items []admincontrol.Announcement) []int {
	ids := make([]int, 0, len(items))
	for _, item := range items {
		if item.ID > 0 {
			ids = append(ids, item.ID)
		}
	}
	return ids
}

func announcementReadByID(reads []admincontrol.AnnouncementRead) map[int]admincontrol.AnnouncementRead {
	out := make(map[int]admincontrol.AnnouncementRead, len(reads))
	for _, read := range reads {
		if read.AnnouncementID <= 0 {
			continue
		}
		out[read.AnnouncementID] = read
	}
	return out
}

type announcementCollection struct {
	NextID int                         `json:"next_id"`
	Items  []admincontrol.Announcement `json:"items"`
}

func announcementFromCreateRequest(req admincontrol.AnnouncementRequest, id int, now time.Time) (admincontrol.Announcement, error) {
	title := strings.TrimSpace(req.Title)
	content := strings.TrimSpace(req.Content)
	if title == "" || content == "" {
		return admincontrol.Announcement{}, admincontrol.ErrInvalidInput
	}
	status := req.Status
	if status == "" {
		status = admincontrol.AnnouncementStatusDraft
	}
	severity := req.Severity
	if severity == "" {
		severity = admincontrol.AnnouncementSeverityInfo
	}
	audience := req.Audience
	if audience == "" {
		audience = admincontrol.AnnouncementAudienceAll
	}
	if !status.Valid() || !severity.Valid() || !audience.Valid() || !validTimeRange(req.StartsAt, req.EndsAt) {
		return admincontrol.Announcement{}, admincontrol.ErrInvalidInput
	}
	return admincontrol.Announcement{
		ID:        id,
		Title:     title,
		Content:   content,
		Status:    status,
		Severity:  severity,
		Audience:  audience,
		Segments:  normalizeAnnouncementSegments(req.Segments),
		StartsAt:  req.StartsAt,
		EndsAt:    req.EndsAt,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// normalizeAnnouncementSegments trims/dedupes each segment's conditions and
// drops segments that carry no condition (which would otherwise match nobody).
func normalizeAnnouncementSegments(segments []admincontrol.AnnouncementSegment) []admincontrol.AnnouncementSegment {
	if len(segments) == 0 {
		return nil
	}
	out := make([]admincontrol.AnnouncementSegment, 0, len(segments))
	for _, seg := range segments {
		roles := uniqueTrimmedStrings(seg.Roles)
		domains := lowerUniqueTrimmedStrings(seg.EmailDomains)
		ids := uniquePositiveInts(seg.UserIDs)
		if len(roles) == 0 && len(domains) == 0 && len(ids) == 0 {
			continue
		}
		out = append(out, admincontrol.AnnouncementSegment{Roles: roles, UserIDs: ids, EmailDomains: domains})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func uniquePositiveInts(values []int) []int {
	out := make([]int, 0, len(values))
	seen := map[int]struct{}{}
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

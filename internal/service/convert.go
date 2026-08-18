// Package service implements the connectRPC handlers, translating between proto
// messages and the data layer. Authorization is enforced by the Casbin
// interceptor (admin bypasses); handlers carry no role checks.
package service

import (
	"time"

	zerxv1 "github.com/zerx-lab/zkit/gen/go/zerx/v1"
	"github.com/zerx-lab/zkit/internal/media"
	"github.com/zerx-lab/zkit/internal/model"
)

// toProtoUser maps a data-layer user to its public proto representation
// (password hash intentionally omitted).
func toProtoUser(u model.User, roles []string, totpEnabled bool, m *media.Media) *zerxv1.User {
	return &zerxv1.User{
		Id:          u.ID,
		Email:       u.Email,
		Name:        u.Name,
		Roles:       roles,
		CreatedAt:   u.CreatedAt.Format(time.RFC3339),
		Nickname:    u.Nickname,
		Avatar:      m.ResolveAvatar(u.Avatar),
		Phone:       u.Phone,
		Status:      u.Status,
		TotpEnabled: totpEnabled,
	}
}

// toProtoSession maps a session row to its proto representation.
func toProtoSession(s model.UserSession) *zerxv1.Session {
	return &zerxv1.Session{
		Id:         s.ID,
		UserId:     s.UserID,
		Ip:         s.IP,
		UserAgent:  s.UserAgent,
		CreatedAt:  s.CreatedAt.Format(time.RFC3339),
		LastSeenAt: s.LastSeenAt.Format(time.RFC3339),
	}
}

// toProtoRole maps a role row to its proto representation.
func toProtoRole(r model.Role) *zerxv1.Role {
	return &zerxv1.Role{
		Id:          r.ID,
		Code:        r.Code,
		Name:        r.Name,
		Description: r.Description,
		Builtin:     r.Builtin,
		Sort:        int32(r.Sort),
		CreatedAt:   r.CreatedAt.Format(time.RFC3339),
	}
}

// toProtoMenuButton maps a button row to its proto representation.
func toProtoMenuButton(b model.MenuButton) *zerxv1.MenuButton {
	return &zerxv1.MenuButton{
		Id:     b.ID,
		MenuId: b.MenuID,
		Code:   b.Code,
		Name:   b.Name,
	}
}

// toProtoMenu maps a menu row to its proto representation with the given
// buttons (children are filled by buildMenuTree).
func toProtoMenu(m model.Menu, buttons []*zerxv1.MenuButton) *zerxv1.Menu {
	return &zerxv1.Menu{
		Id:        m.ID,
		ParentId:  m.ParentID,
		Path:      m.Path,
		Name:      m.Name,
		Component: m.Component,
		Title:     m.Title,
		Icon:      m.Icon,
		Sort:      int32(m.Sort),
		Hidden:    m.Hidden,
		Buttons:   buttons,
		CreatedAt: m.CreatedAt.Format(time.RFC3339),
	}
}

// buildMenuTree assembles a nested menu tree rooted at parentID from a flat,
// sorted slice of menus and their buttons.
func buildMenuTree(menus []model.Menu, buttons []model.MenuButton, parentID uint64) []*zerxv1.Menu {
	buttonsByMenu := make(map[uint64][]*zerxv1.MenuButton)
	for i := range buttons {
		buttonsByMenu[buttons[i].MenuID] = append(buttonsByMenu[buttons[i].MenuID], toProtoMenuButton(buttons[i]))
	}

	var build func(parent uint64) []*zerxv1.Menu
	build = func(parent uint64) []*zerxv1.Menu {
		var out []*zerxv1.Menu
		for i := range menus {
			if menus[i].ParentID != parent {
				continue
			}
			node := toProtoMenu(menus[i], buttonsByMenu[menus[i].ID])
			node.Children = build(menus[i].ID)
			out = append(out, node)
		}

		return out
	}

	return build(parentID)
}

// toProtoAPI maps an API catalog row to its proto representation.
func toProtoAPI(a model.API) *zerxv1.Api {
	return &zerxv1.Api{
		Id:          a.ID,
		Procedure:   a.Procedure,
		Service:     a.Service,
		Method:      a.Method,
		Description: a.Description,
		Group:       a.Group,
		CreatedAt:   a.CreatedAt.Format(time.RFC3339),
	}
}

// toProtoDict maps a dictionary row to its proto representation.
func toProtoDict(d model.Dictionary) *zerxv1.Dict {
	return &zerxv1.Dict{
		Id:          d.ID,
		Type:        d.Type,
		Name:        d.Name,
		Description: d.Description,
		Status:      d.Status,
		CreatedAt:   d.CreatedAt.Format(time.RFC3339),
	}
}

// toProtoDictItem maps a dictionary item row to its proto representation.
func toProtoDictItem(i model.DictionaryItem) *zerxv1.DictItem {
	return &zerxv1.DictItem{
		Id:     i.ID,
		DictId: i.DictID,
		Label:  i.Label,
		Value:  i.Value,
		Sort:   int32(i.Sort),
		Status: i.Status,
	}
}

// toProtoSysParam maps a system parameter row to its proto representation.
func toProtoSysParam(p model.SysParam) *zerxv1.SysParam {
	return &zerxv1.SysParam{
		Id:          p.ID,
		Key:         p.Key,
		Name:        p.Name,
		Value:       p.Value,
		Description: p.Description,
		CreatedAt:   p.CreatedAt.Format(time.RFC3339),
	}
}

// toProtoFile maps a file row to its proto representation.
func toProtoFile(f model.File, m *media.Media) *zerxv1.File {
	return &zerxv1.File{
		Id:          f.ID,
		Name:        f.Name,
		Key:         f.Key,
		Url:         m.ResolveFile(f.Key, f.Visibility),
		Size:        f.Size,
		ContentType: f.ContentType,
		UploadedBy:  f.UploadedBy,
		CreatedAt:   f.CreatedAt.Format(time.RFC3339),
		Visibility:  f.Visibility,
	}
}

// toProtoOperationLog maps an operation log row to its proto representation.
func toProtoOperationLog(o model.OperationLog) *zerxv1.OperationLog {
	return &zerxv1.OperationLog{
		Id:        o.ID,
		UserId:    o.UserID,
		UserEmail: o.UserEmail,
		Procedure: o.Procedure,
		Method:    o.Method,
		Ip:        o.IP,
		UserAgent: o.UserAgent,
		LatencyMs: o.LatencyMS,
		Status:    o.Status,
		Error:     o.Error,
		Stack:     o.Stack,
		CreatedAt: o.CreatedAt.Format(time.RFC3339),
		Detail:    o.Detail,
	}
}

// toProtoLoginLog maps a login log row to its proto representation.
func toProtoLoginLog(l model.LoginLog) *zerxv1.LoginLog {
	return &zerxv1.LoginLog{
		Id:        l.ID,
		UserId:    l.UserID,
		Email:     l.Email,
		Ip:        l.IP,
		UserAgent: l.UserAgent,
		Success:   l.Success,
		Error:     l.Error,
		CreatedAt: l.CreatedAt.Format(time.RFC3339),
	}
}

// normalizePage clamps page/pageSize to sane bounds and returns 1-based page
// plus the SQL offset.
func normalizePage(page, pageSize int32) (p, ps, offset int) {
	p = int(page)
	if p < 1 {
		p = 1
	}
	ps = int(pageSize)
	if ps < 1 || ps > maxPageSize {
		ps = defaultPageSize
	}

	return p, ps, (p - 1) * ps
}

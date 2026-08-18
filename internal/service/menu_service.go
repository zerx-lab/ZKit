package service

import (
	"context"
	"errors"
	"slices"

	"connectrpc.com/connect"
	"gorm.io/gorm"

	zerxv1 "github.com/zerx-lab/zkit/gen/go/zerx/v1"
	"github.com/zerx-lab/zkit/gen/go/zerx/v1/zerxv1connect"
	"github.com/zerx-lab/zkit/internal/audit"
	"github.com/zerx-lab/zkit/internal/auth"
	"github.com/zerx-lab/zkit/internal/model"
	"github.com/zerx-lab/zkit/internal/plugin"
)

// MenuService implements zerxv1connect.MenuServiceHandler.
type MenuService struct {
	db          *gorm.DB
	pluginState *plugin.State
}

var _ zerxv1connect.MenuServiceHandler = (*MenuService)(nil)

// NewMenuService constructs the menu handler. pluginState filters menus owned by
// disabled plugins out of GetUserMenus (nil = no filtering, e.g. in tests).
func NewMenuService(db *gorm.DB, pluginState *plugin.State) *MenuService {
	return &MenuService{db: db, pluginState: pluginState}
}

// keepMenu reports whether a menu row should be surfaced to users: always true
// for core menus, and for plugin menus (name prefixed "plg_<plugin>") only when
// the owning plugin is enabled.
func (s *MenuService) keepMenu(m model.Menu) bool {
	if s.pluginState == nil {
		return true
	}
	name := plugin.PluginNameOfMenu(m.Name)
	if name == "" {
		return true
	}
	return s.pluginState.IsEnabled(name)
}

func (s *MenuService) ListMenus(ctx context.Context, _ *connect.Request[zerxv1.ListMenusRequest]) (*connect.Response[zerxv1.ListMenusResponse], error) {
	menus, buttons, err := s.loadMenusAndButtons(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&zerxv1.ListMenusResponse{Menus: buildMenuTree(menus, buttons, 0)}), nil
}

func (s *MenuService) CreateMenu(ctx context.Context, req *connect.Request[zerxv1.CreateMenuRequest]) (*connect.Response[zerxv1.Menu], error) {
	m := model.Menu{
		ParentID:  req.Msg.GetParentId(),
		Path:      req.Msg.GetPath(),
		Name:      req.Msg.GetName(),
		Component: req.Msg.GetComponent(),
		Title:     req.Msg.GetTitle(),
		Icon:      req.Msg.GetIcon(),
		Sort:      int(req.Msg.GetSort()),
		Hidden:    req.Msg.GetHidden(),
	}
	if err := gorm.G[model.Menu](s.db).Create(ctx, &m); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	audit.Record(ctx, auditJSON(map[string]any{"after": map[string]any{"id": m.ID, "parent_id": m.ParentID, "path": m.Path, "name": m.Name, "title": m.Title}}))
	return connect.NewResponse(toProtoMenu(m, nil)), nil
}

func (s *MenuService) UpdateMenu(ctx context.Context, req *connect.Request[zerxv1.UpdateMenuRequest]) (*connect.Response[zerxv1.Menu], error) {
	id := req.Msg.GetId()
	old, err := gorm.G[model.Menu](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("menu not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	updates := map[string]any{
		"parent_id": req.Msg.GetParentId(),
		"path":      req.Msg.GetPath(),
		"name":      req.Msg.GetName(),
		"component": req.Msg.GetComponent(),
		"title":     req.Msg.GetTitle(),
		"icon":      req.Msg.GetIcon(),
		"sort":      int(req.Msg.GetSort()),
		"hidden":    req.Msg.GetHidden(),
	}
	if err := s.db.WithContext(ctx).Model(&model.Menu{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	m, err := gorm.G[model.Menu](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	audit.Record(ctx, auditJSON(map[string]any{"before": map[string]any{"id": old.ID, "parent_id": old.ParentID, "path": old.Path, "name": old.Name, "title": old.Title}, "after": map[string]any{"id": m.ID, "parent_id": m.ParentID, "path": m.Path, "name": m.Name, "title": m.Title}}))
	return connect.NewResponse(toProtoMenu(m, nil)), nil
}

func (s *MenuService) DeleteMenu(ctx context.Context, req *connect.Request[zerxv1.DeleteMenuRequest]) (*connect.Response[zerxv1.DeleteMenuResponse], error) {
	id := req.Msg.GetId()
	old, err := gorm.G[model.Menu](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("menu not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("menu_id = ?", id).Delete(&model.MenuButton{}).Error; err != nil {
			return err
		}
		if err := tx.Where("menu_id = ?", id).Delete(&model.RoleMenu{}).Error; err != nil {
			return err
		}
		res := tx.Where("id = ?", id).Delete(&model.Menu{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		return nil
	})
	if errors.Is(txErr, gorm.ErrRecordNotFound) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("menu not found"))
	}
	if txErr != nil {
		return nil, connect.NewError(connect.CodeInternal, txErr)
	}

	audit.Record(ctx, auditJSON(map[string]any{"before": map[string]any{"id": old.ID, "name": old.Name, "path": old.Path, "title": old.Title}}))
	return connect.NewResponse(&zerxv1.DeleteMenuResponse{}), nil
}

func (s *MenuService) CreateMenuButton(ctx context.Context, req *connect.Request[zerxv1.CreateMenuButtonRequest]) (*connect.Response[zerxv1.MenuButton], error) {
	b := model.MenuButton{
		MenuID: req.Msg.GetMenuId(),
		Code:   req.Msg.GetCode(),
		Name:   req.Msg.GetName(),
	}
	if err := gorm.G[model.MenuButton](s.db).Create(ctx, &b); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	audit.Record(ctx, auditJSON(map[string]any{"after": map[string]any{"id": b.ID, "menu_id": b.MenuID, "code": b.Code, "name": b.Name}}))
	return connect.NewResponse(toProtoMenuButton(b)), nil
}

func (s *MenuService) DeleteMenuButton(ctx context.Context, req *connect.Request[zerxv1.DeleteMenuButtonRequest]) (*connect.Response[zerxv1.DeleteMenuButtonResponse], error) {
	id := req.Msg.GetId()
	oldBtn, btnErr := gorm.G[model.MenuButton](s.db).Where("id = ?", id).First(ctx)
	if btnErr != nil {
		if errors.Is(btnErr, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("button not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, btnErr)
	}
	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("button_id = ?", id).Delete(&model.RoleButton{}).Error; err != nil {
			return err
		}
		res := tx.Where("id = ?", id).Delete(&model.MenuButton{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		return nil
	})
	if errors.Is(txErr, gorm.ErrRecordNotFound) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("button not found"))
	}
	if txErr != nil {
		return nil, connect.NewError(connect.CodeInternal, txErr)
	}

	audit.Record(ctx, auditJSON(map[string]any{"before": map[string]any{"id": oldBtn.ID, "menu_id": oldBtn.MenuID, "code": oldBtn.Code, "name": oldBtn.Name}}))
	return connect.NewResponse(&zerxv1.DeleteMenuButtonResponse{}), nil
}

func (s *MenuService) GetUserMenus(ctx context.Context, _ *connect.Request[zerxv1.GetUserMenusRequest]) (*connect.Response[zerxv1.GetUserMenusResponse], error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}

	menus, buttons, err := s.loadMenusAndButtons(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Drop menus owned by disabled plugins (applies to admin too: a disabled
	// plugin is fully hidden).
	if s.pluginState != nil {
		filtered := menus[:0]
		for i := range menus {
			if s.keepMenu(menus[i]) {
				filtered = append(filtered, menus[i])
			}
		}
		menus = filtered
	}

	if slices.Contains(claims.Roles, model.RoleAdmin) {
		return connect.NewResponse(&zerxv1.GetUserMenusResponse{Menus: buildMenuTree(menus, buttons, 0)}), nil
	}

	roleMenus, err := gorm.G[model.RoleMenu](s.db).Where("role_code IN ?", claims.Roles).Find(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	granted := make(map[uint64]bool, len(roleMenus))
	for i := range roleMenus {
		granted[roleMenus[i].MenuID] = true
	}

	// Auto-include ancestor group nodes so granted leaves are not orphaned.
	byID := make(map[uint64]model.Menu, len(menus))
	for i := range menus {
		byID[menus[i].ID] = menus[i]
	}
	for _, id := range maps2keys(granted) {
		cur := byID[id]
		for cur.ParentID != 0 {
			if granted[cur.ParentID] {
				break
			}
			granted[cur.ParentID] = true
			cur = byID[cur.ParentID]
		}
	}

	visible := make([]model.Menu, 0, len(granted))
	for i := range menus {
		if granted[menus[i].ID] {
			visible = append(visible, menus[i])
		}
	}

	return connect.NewResponse(&zerxv1.GetUserMenusResponse{Menus: buildMenuTree(visible, buttons, 0)}), nil
}

func (s *MenuService) GetUserButtons(ctx context.Context, _ *connect.Request[zerxv1.GetUserButtonsRequest]) (*connect.Response[zerxv1.GetUserButtonsResponse], error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}

	if slices.Contains(claims.Roles, model.RoleAdmin) {
		buttons, err := gorm.G[model.MenuButton](s.db).Find(ctx)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		codes := make([]string, 0, len(buttons))
		for i := range buttons {
			codes = append(codes, buttons[i].Code)
		}

		return connect.NewResponse(&zerxv1.GetUserButtonsResponse{Codes: codes}), nil
	}

	roleButtons, err := gorm.G[model.RoleButton](s.db).Where("role_code IN ?", claims.Roles).Find(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	ids := make([]uint64, 0, len(roleButtons))
	for i := range roleButtons {
		ids = append(ids, roleButtons[i].ButtonID)
	}
	if len(ids) == 0 {
		return connect.NewResponse(&zerxv1.GetUserButtonsResponse{}), nil
	}

	buttons, err := gorm.G[model.MenuButton](s.db).Where("id IN ?", ids).Find(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	codes := make([]string, 0, len(buttons))
	for i := range buttons {
		codes = append(codes, buttons[i].Code)
	}

	return connect.NewResponse(&zerxv1.GetUserButtonsResponse{Codes: codes}), nil
}

func (s *MenuService) loadMenusAndButtons(ctx context.Context) ([]model.Menu, []model.MenuButton, error) {
	menus, err := gorm.G[model.Menu](s.db).Order("sort ASC, id ASC").Find(ctx)
	if err != nil {
		return nil, nil, err
	}
	buttons, err := gorm.G[model.MenuButton](s.db).Order("id ASC").Find(ctx)
	if err != nil {
		return nil, nil, err
	}

	return menus, buttons, nil
}

func maps2keys(m map[uint64]bool) []uint64 {
	out := make([]uint64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	return out
}

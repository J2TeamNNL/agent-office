package transfer

import (
	"context"
	"errors"
	"fmt"
	"github.com/tung1998/agent-office/templates/packs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/tung1998/agent-office/internal/perm"
	"github.com/tung1998/agent-office/internal/repos"
	"github.com/tung1998/agent-office/internal/storage"
	"github.com/tung1998/agent-office/internal/team"
)

// SetCloneRoot is where a new project's folder goes (cloned or empty).
func (s *Service) SetCloneRoot(dir string) { s.cloneRoot = dir }

// CloneRoot is where a new project's folder goes.
func (s *Service) CloneRoot() string {
	if s.cloneRoot != "" {
		return s.cloneRoot
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "/"
}

var dirRe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// DirName is a folder name for a project called name.
func DirName(name string) string {
	return strings.Trim(dirRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-"), "-.")
}

// CheckNewProject checks np before a person approves it, and fills its pack's
// name, description and git remote when it imports one.
func (s *Service) CheckNewProject(ctx context.Context, np *storage.NewProject) error {
	if np.Pack == "" && np.Path != "" { // a folder carrying its own pack (ADR-190)
		if f := filepath.Join(np.Path, RepoPackFile); fileExists(f) {
			np.Pack = f
		}
	}
	if np.Pack != "" {
		p, err := readPack(np.Pack)
		if err != nil {
			return err
		}
		if packs.IsRef(np.Pack) && (np.URL != "" || np.Path != "") {
			return errors.New("mẫu của thư viện tạo repo mới: bỏ url và path")
		}
		np.Name = orStr(strings.TrimSpace(np.Name), p.Project.Name)
		np.Description = orStr(np.Description, p.Project.Description)
		if np.URL == "" && np.Path == "" {
			np.URL = p.Project.GitRemote
		}
		np.Include = CleanIncludes(orSlice(np.Include, p.Include))
	}
	switch {
	case np.URL != "" && np.Path != "":
		return errors.New("chọn một: clone từ url, hoặc thư mục có sẵn (path)")
	case np.URL != "":
		if _, err := repos.CloneURL(np.URL); err != nil {
			return err
		}
	case np.Path != "":
		if !filepath.IsAbs(np.Path) {
			return errors.New("path phải là đường dẫn tuyệt đối")
		}
		info, err := repos.Detect(np.Path) // as it is kept: absolute, links resolved
		if err != nil {
			return fmt.Errorf("không có thư mục %s", np.Path)
		}
		if _, err := s.store.Repos().GetByPath(ctx, info.Path); err == nil {
			return fmt.Errorf("thư mục %s đã là một project", np.Path)
		}
	default:
		if strings.TrimSpace(np.Name) == "" && np.Dir == "" {
			return errors.New("project mới cần tên (hoặc url để clone, path có sẵn)")
		}
	}
	return nil
}

func orSlice(a, b []string) []string {
	if len(a) > 0 {
		return a
	}
	return b
}

// CreateProject makes np (ADR-170): clones its URL, takes its Path, or
// starts an empty repo; then imports its pack, or gives it the starter agent.
func (s *Service) CreateProject(ctx context.Context, np storage.NewProject) (storage.Repo, error) {
	if err := s.CheckNewProject(ctx, &np); err != nil {
		return storage.Repo{}, err
	}
	var pack *Pack
	if np.Pack != "" {
		p, err := readPack(np.Pack)
		if err != nil {
			return storage.Repo{}, err
		}
		pack = &p
	}
	dir, made := np.Path, false
	switch {
	case np.URL != "":
		d, err := repos.Clone(ctx, np.URL, s.CloneRoot(), np.Dir)
		if err != nil {
			return storage.Repo{}, err
		}
		dir, made = d, true
	case np.Path == "":
		d, err := repos.Init(ctx, s.CloneRoot(), orStr(np.Dir, DirName(np.Name)))
		if err != nil {
			return storage.Repo{}, err
		}
		dir, made = d, true
	}
	undo := func() {
		if made {
			_ = os.RemoveAll(dir)
		}
	}
	if packs.IsRef(np.Pack) { // the new repo starts with the library pack, committed (ADR-199)
		if _, err := packs.Write(ctx, np.Pack, dir, true); err != nil {
			undo()
			return storage.Repo{}, err
		}
	}
	info, err := repos.Detect(dir)
	if err != nil {
		undo()
		return storage.Repo{}, err
	}
	if pack == nil && np.URL != "" { // a clone carrying its own pack
		p, err := RepoPack(dir)
		if err != nil {
			undo()
			return storage.Repo{}, err
		}
		if p != nil {
			pack, np.Include = p, p.Include
			np.Name, np.Description = orStr(strings.TrimSpace(np.Name), p.Project.Name), orStr(np.Description, p.Project.Description)
		}
	}
	r, err := s.store.Repos().Create(ctx, storage.Repo{Name: orStr(strings.TrimSpace(np.Name), info.Name), Path: info.Path,
		GitRemote: info.GitRemote, Description: orStr(np.Description, info.Description)})
	if err != nil {
		undo()
		return storage.Repo{}, err
	}
	if pack != nil {
		_, err = s.ImportPack(ctx, *pack, r, np.Include)
	} else if s.team != nil {
		err = s.team.AddFromLibrary(ctx, r.ID, team.StarterAgent)
	}
	if err == nil && np.Parent != "" {
		err = s.attach(ctx, r, np)
	}
	if err != nil {
		_ = s.store.Repos().Delete(ctx, r.ID)
		undo()
		return storage.Repo{}, err
	}
	return r, nil
}

// attach puts project r under np.Parent (ADR-191): the two link to each
// other, and each default agent gets a note of who is above or below it.
func (s *Service) attach(ctx context.Context, r storage.Repo, np storage.NewProject) error {
	parent, err := s.store.Repos().Get(ctx, np.Parent)
	if err != nil {
		return fmt.Errorf("không có project cấp trên %q", np.Parent)
	}
	link := func(from, to string) error {
		_, err := perm.UpdatePolicy(ctx, s.store, from, func(p *perm.Policy) (bool, error) {
			if slices.Contains(p.LinkedProjects, to) {
				return false, nil
			}
			p.LinkedProjects = append(p.LinkedProjects, to)
			return true, nil
		})
		return err
	}
	if err := link(parent.ID, r.ID); err != nil {
		return err
	}
	if err := link(r.ID, parent.ID); err != nil {
		return err
	}
	up := fmt.Sprintf("Cấp trên: project %s (id %s). Chỉ đạo đến từ chat do cấp trên mở ở đây: làm xong thì trả lời ngay trong chat đó, câu trả lời tự về cấp trên (báo cáo lên). Việc gấp cần cấp trên biết: send_to_chat project=%s.", parent.Name, parent.ID, parent.Name)
	switch {
	case packs.IsRef(np.Pack):
		up += " Mẫu của đơn vị: " + np.Pack + " (thư viện mẫu của office)."
	case np.Pack != "":
		up += " Gói mẫu của tổ chức ở " + filepath.Dir(np.Pack) + "."
	}
	if err := s.note(ctx, r, up, "", ""); err != nil {
		return err
	}
	down := fmt.Sprintf("Đơn vị cấp dưới: project %s (id %s)", r.Name, r.ID)
	if r.Description != "" {
		down += ": " + r.Description
	}
	return s.note(ctx, parent, down+". Giao việc và hỏi báo cáo: send_to_chat project="+r.Name+".", "cap-duoi", "Các project cấp dưới và nhiệm vụ của chúng")
}

// note adds a note to r's default agent.
func (s *Service) note(ctx context.Context, r storage.Repo, text, topic, summary string) error {
	r, err := s.store.Repos().Get(ctx, r.ID) // its default agent as it is now
	if err != nil {
		return err
	}
	agents, err := s.store.Agents().List(ctx, r.ID)
	if err != nil {
		return err
	}
	a, ok := storage.DefaultAgent(r, agents)
	if !ok {
		return nil
	}
	_, err = s.store.Memories().Create(ctx, storage.Memory{ProjectID: r.ID, AgentID: a.ID, Text: text, Source: "office", Topic: topic, Summary: summary, CreatedBy: "office"})
	return err
}

// RepoPack reads the pack a folder carries (RepoPackFile); nil when it has none.
func RepoPack(dir string) (*Pack, error) {
	f := filepath.Join(dir, RepoPackFile)
	if dir == "" || !fileExists(f) {
		return nil, nil
	}
	p, err := ReadPackFile(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", RepoPackFile, err)
	}
	return &p, nil
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// ReadPackFile reads a pack from a .zip or its JSON on this machine.
// readPack is a pack file, or a pack of office's library by name (ADR-199).
func readPack(src string) (Pack, error) {
	if !packs.IsRef(src) {
		return ReadPackFile(src)
	}
	raw, err := packs.Read(src)
	if err != nil {
		return Pack{}, err
	}
	return ReadPack(raw)
}

func ReadPackFile(path string) (Pack, error) {
	st, err := os.Stat(path)
	if err != nil {
		return Pack{}, fmt.Errorf("không đọc được %s", path)
	}
	if st.Size() > MaxPack {
		return Pack{}, errors.New("gói project quá lớn")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Pack{}, err
	}
	return ReadPack(data)
}

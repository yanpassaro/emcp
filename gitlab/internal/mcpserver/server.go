package mcpserver

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"ntdsk.com/gitlab/internal/gitlab"
)

const (
	MAX_FILE_BYTES     = 200 * 1024
	MAX_DIFF_BYTES     = 200 * 1024
	MAX_DIFF_ITEMS     = 300
	MAX_FILES_PER_CALL = 20
	MAX_PER_PAGE       = 100
	DEFAULT_PER_PAGE   = 50
	MAX_TREE_ITEMS     = 1000
	KILOBYTE           = 1024
	MEGABYTE           = 1024 * 1024
	FILE_TRUNCATED_MSG = " (truncado em 200KB)"
)

type Server struct {
	client *gitlab.Client
}

func New(client *gitlab.Client) *Server {
	return &Server{client: client}
}

func (s *Server) Register(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_search",
		Description: "Search GitLab resources. Scopes: projects, issues, merge_requests, milestones, users, commits, notes, wiki_blobs, blobs.",
	}, s.search)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list",
		Description: "List GitLab resources. Set 'entity' (projects|users|issues|merge_requests|pipelines|branches|commits|members|tags|jobs|tree). Filters vary per entity; pipelines/branches/commits/tags/jobs/tree require project_id (numeric id or group/project path). Tree filters: path, ref, recursive.",
	}, s.list)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_item",
		Description: "Get a single GitLab item by id. entity = project|user|issue|merge_request|pipeline|branch|commit|member|tag|job. Many entities require project_id (numeric id or group/project path).",
	}, s.getItem)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_file",
		Description: "Get file content from a repository (project_id). Use 'path' for a single file (optional line_start/line_end) or 'paths' for up to 20 files. ref = branch/tag/commit, default is the default branch. Files over 200KB are truncated.",
	}, s.getFile)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_diff",
		Description: "Get a diff: a merge request's changes (merge_request_iid), a commit's diff (sha) or a comparison between refs (from/to).",
	}, s.getDiff)
}

type GetFileInput struct {
	ProjectID string   `json:"project_id" jsonschema:"Project ID or path (e.g. 42 or group/project)"`
	Path      string   `json:"path,omitempty" jsonschema:"File path in the repository (e.g. src/main.go)"`
	Paths     []string `json:"paths,omitempty" jsonschema:"Several file paths (up to 20); use instead of path"`
	Ref       string   `json:"ref,omitempty" jsonschema:"Branch/tag/commit (default: default branch)"`
	LineStart *int     `json:"line_start,omitempty" jsonschema:"First line to return (1-based, only with path)"`
	LineEnd   *int     `json:"line_end,omitempty" jsonschema:"Last line to return (1-based, inclusive, only with path)"`
}

type GetDiffInput struct {
	ProjectID string `json:"project_id" jsonschema:"Project ID or path (e.g. 42 or group/project)"`
	MRIID     *int   `json:"merge_request_iid,omitempty" jsonschema:"Merge request IID (changes of that MR)"`
	SHA       string `json:"sha,omitempty" jsonschema:"Commit SHA (diff of that commit)"`
	From      string `json:"from,omitempty" jsonschema:"Compare: base ref (default: default branch)"`
	To        string `json:"to,omitempty" jsonschema:"Compare: head ref (default: default branch)"`
}

func validID(p *int) bool {
	if p == nil {
		return false
	}
	return *p > 0
}

func requireProject(project string) error {
	if strings.TrimSpace(project) == "" {
		return errors.New("project_id é obrigatório")
	}
	return nil
}

func hasProject(project string) bool {
	return strings.TrimSpace(project) != ""
}

func projectPath(project string) string {
	return url.PathEscape(strings.TrimSpace(project))
}

func projectEndpoint(project, suffix string) string {
	return fmt.Sprintf("/projects/%s%s", projectPath(project), suffix)
}

func cleanPaths(paths []string) []string {
	out := []string{}
	for _, p := range paths {
		path := strings.TrimSpace(p)
		if path != "" {
			out = append(out, path)
		}
	}
	return out
}

func optionalID(query url.Values, key string, p *int) {
	if validID(p) {
		query.Set(key, strconv.Itoa(*p))
	}
}

func setOrder(query url.Values, orderBy, sort string) {
	if orderBy == "" {
		return
	}
	query.Set("order_by", orderBy)
	query.Set("sort", sort)
}

func setText(query url.Values, key, value string) {
	if value == "" {
		return
	}
	query.Set(key, value)
}

func scopedEndpoint(fallback, projectSuffix, groupSuffix, project string, groupID *int) string {
	if hasProject(project) {
		return projectEndpoint(project, projectSuffix)
	}
	if validID(groupID) {
		return fmt.Sprintf(groupSuffix, *groupID)
	}
	return fallback
}

func memberEndpoint(project string, groupID *int, id string) string {
	if hasProject(project) {
		return projectEndpoint(project, fmt.Sprintf("/members/%s", id))
	}
	if validID(groupID) {
		return fmt.Sprintf("/groups/%d/members/%s", *groupID, id)
	}
	return ""
}

func jobsRequest(project string, pipelineID *int, scope string) (string, url.Values) {
	query := url.Values{}
	if validID(pipelineID) {
		return projectEndpoint(project, fmt.Sprintf("/pipelines/%d/jobs", *pipelineID)), query
	}

	setText(query, "scope", scope)
	return projectEndpoint(project, "/jobs"), query
}

func (s *Server) defaultBranch(ctx context.Context, project string) string {
	raw, err := s.client.Get(ctx, projectEndpoint(project, ""), url.Values{})
	if err != nil {
		return ""
	}

	m, ok := raw.(map[string]any)
	if !ok {
		return ""
	}

	db, ok := m["default_branch"].(string)
	if !ok {
		return ""
	}

	return db
}

func (s *Server) projectID(ctx context.Context, project string) (int, error) {
	id, err := strconv.Atoi(strings.TrimSpace(project))
	if err == nil {
		return id, nil
	}

	raw, err := s.client.Get(ctx, projectEndpoint(project, ""), url.Values{})
	if err != nil {
		return 0, err
	}

	m, ok := raw.(map[string]any)
	if !ok {
		return 0, errors.New("resposta inesperada do GitLab")
	}

	id = toInt(m["id"])
	if id <= 0 {
		return 0, errors.New("project_id não encontrado")
	}

	return id, nil
}

func fileContent(f map[string]any) string {
	b64, ok := f["content"].(string)
	if !ok {
		return ""
	}

	dec, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return ""
	}

	return string(dec)
}

func (s *Server) getFile(ctx context.Context, _ *mcp.CallToolRequest, input GetFileInput) (*mcp.CallToolResult, any, error) {
	if err := requireProject(input.ProjectID); err != nil {
		return nil, nil, err
	}

	path := strings.TrimSpace(input.Path)
	paths := cleanPaths(input.Paths)

	if path != "" {
		if len(paths) != 0 {
			return nil, nil, errors.New("informe path ou paths, não ambos")
		}

		start := 0
		if validID(input.LineStart) {
			start = *input.LineStart
		}

		end := 0
		if validID(input.LineEnd) {
			end = *input.LineEnd
		}

		block, err := s.fileBlock(ctx, input.ProjectID, path, strings.TrimSpace(input.Ref), start, end)
		if err != nil {
			return nil, nil, err
		}

		return textResult(block)
	}

	if len(paths) == 0 {
		return nil, nil, errors.New("path ou paths é obrigatório")
	}

	if len(paths) > MAX_FILES_PER_CALL {
		return nil, nil, fmt.Errorf("no máximo %d arquivos por chamada", MAX_FILES_PER_CALL)
	}

	if validID(input.LineStart) {
		return nil, nil, errors.New("line_start só se aplica a path único")
	}

	if validID(input.LineEnd) {
		return nil, nil, errors.New("line_end só se aplica a path único")
	}

	ref := strings.TrimSpace(input.Ref)
	if ref == "" {
		ref = s.defaultBranch(ctx, input.ProjectID)
	}

	b := strings.Builder{}
	for _, p := range paths {
		block, err := s.fileBlock(ctx, input.ProjectID, p, ref, 0, 0)
		if err != nil {
			fmt.Fprintf(&b, "❌ %s: %v\n\n", p, err)
			continue
		}

		b.WriteString(block)
		b.WriteString("\n\n")
	}

	return textResult(strings.TrimSpace(b.String()))
}

func (s *Server) getDiff(ctx context.Context, _ *mcp.CallToolRequest, input GetDiffInput) (*mcp.CallToolResult, any, error) {
	if err := requireProject(input.ProjectID); err != nil {
		return nil, nil, err
	}

	from := strings.TrimSpace(input.From)
	to := strings.TrimSpace(input.To)
	sha := strings.TrimSpace(input.SHA)

	hasCompare := false
	if from != "" {
		hasCompare = true
	}
	if to != "" {
		hasCompare = true
	}

	hasSHA := sha != ""
	hasMR := validID(input.MRIID)

	modes := 0
	if hasMR {
		modes++
	}
	if hasSHA {
		modes++
	}
	if hasCompare {
		modes++
	}

	if modes == 0 {
		return nil, nil, errors.New("informe merge_request_iid, sha ou from/to")
	}

	if modes > 1 {
		return nil, nil, errors.New("informe apenas um modo: merge_request_iid, sha ou from/to")
	}

	query := url.Values{}
	endpoint := ""

	switch {
	case hasMR:
		endpoint = projectEndpoint(input.ProjectID, fmt.Sprintf("/merge_requests/%d/changes", *input.MRIID))
	case hasSHA:
		endpoint = projectEndpoint(input.ProjectID, fmt.Sprintf("/repository/commits/%s/diff", url.PathEscape(sha)))
	default:
		setText(query, "from", from)
		setText(query, "to", to)
		endpoint = projectEndpoint(input.ProjectID, "/repository/compare")
	}

	if hasSHA {
		items, err := s.client.List(ctx, endpoint, query, MAX_DIFF_ITEMS)
		if err != nil {
			return nil, nil, err
		}
		return textResult(formatDiff(items, MAX_DIFF_BYTES))
	}

	raw, err := s.client.Get(ctx, endpoint, query)
	if err != nil {
		return nil, nil, err
	}

	return textResult(formatDiff(raw, MAX_DIFF_BYTES))
}

func (s *Server) fileBlock(ctx context.Context, project, path, ref string, start, end int) (string, error) {
	if ref == "" {
		ref = s.defaultBranch(ctx, project)
	}

	query := url.Values{}
	query.Set("ref", ref)

	endpoint := projectEndpoint(project, fmt.Sprintf("/repository/files/%s", url.PathEscape(path)))
	raw, err := s.client.Get(ctx, endpoint, query)
	if err != nil {
		return "", err
	}

	f, ok := raw.(map[string]any)
	if !ok {
		return "", errors.New("resposta inesperada do GitLab")
	}

	content := lineRange(fileContent(f), start, end)
	size := len(content)
	truncated := size > MAX_FILE_BYTES
	if truncated {
		content = content[:MAX_FILE_BYTES]
	}

	b := strings.Builder{}
	fmt.Fprintf(&b, "📄 %s", path)

	fileRef, ok := f["ref"].(string)
	if ok {
		if fileRef != "" {
			fmt.Fprintf(&b, " (@%s)", fileRef)
		}
	}

	if truncated {
		b.WriteString(FILE_TRUNCATED_MSG)
	}

	fmt.Fprintf(&b, "\n\n```%s\n", langFromPath(path))
	b.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("```")

	if truncated {
		b.WriteString("\n\n⚠️ Arquivo maior que 200KB: conteúdo truncado. Use line_start/line_end para ler outra faixa do arquivo.")
	}

	return strings.TrimSpace(b.String()), nil
}

func lineRange(content string, start, end int) string {
	if start <= 0 {
		return content
	}

	lines := strings.Split(content, "\n")
	if start > len(lines) {
		return ""
	}

	last := len(lines)
	if end > 0 {
		if end < last {
			last = end
		}
	}

	if last < start {
		return ""
	}

	return strings.Join(lines[start-1:last], "\n")
}

func humanSize(n int) string {
	switch {
	case n >= MEGABYTE:
		return fmt.Sprintf("%.1f MB", float64(n)/MEGABYTE)
	case n >= KILOBYTE:
		return fmt.Sprintf("%.1f KB", float64(n)/KILOBYTE)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func perPage(p *int) int {
	if validID(p) {
		if *p > MAX_PER_PAGE {
			return MAX_PER_PAGE
		}
		return *p
	}
	return DEFAULT_PER_PAGE
}

func textResult(text string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}, nil, nil
}

type SearchInput struct {
	Scope     string `json:"scope" jsonschema:"Search scope: projects, issues, merge_requests, milestones, users, commits, notes, wiki_blobs or blobs"`
	Search    string `json:"search" jsonschema:"Search term"`
	ProjectID string `json:"project_id,omitempty" jsonschema:"Restrict to a project ID or path (required for some scopes, e.g. commits)"`
	GroupID   *int   `json:"group_id,omitempty" jsonschema:"Restrict to a group"`
	PerPage   *int   `json:"per_page,omitempty" jsonschema:"Items per page (1-100, default 50)"`
}

func (s *Server) search(ctx context.Context, _ *mcp.CallToolRequest, input SearchInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.Scope) == "" {
		return nil, nil, errors.New("scope é obrigatório")
	}

	if strings.TrimSpace(input.Search) == "" {
		return nil, nil, errors.New("search é obrigatório")
	}

	query := url.Values{}
	query.Set("scope", strings.TrimSpace(input.Scope))
	query.Set("search", strings.TrimSpace(input.Search))

	if hasProject(input.ProjectID) {
		id, err := s.projectID(ctx, input.ProjectID)
		if err != nil {
			return nil, nil, err
		}
		query.Set("project_id", strconv.Itoa(id))
	}

	optionalID(query, "group_id", input.GroupID)

	items, err := s.client.List(ctx, "/search", query, perPage(input.PerPage))
	if err != nil {
		return nil, nil, err
	}
	return textResult(formatSearchTable(input.Scope, items))
}

type ListProjectsInput struct {
	Search     string `json:"search,omitempty" jsonschema:"Search by project name"`
	Membership *bool  `json:"membership,omitempty" jsonschema:"Only projects you're a member of (default: true)"`
	Owned      *bool  `json:"owned,omitempty" jsonschema:"Only projects you own"`
	Archived   *bool  `json:"archived,omitempty" jsonschema:"Include archived"`
	Starred    *bool  `json:"starred,omitempty" jsonschema:"Only starred projects"`
	OrderBy    string `json:"order_by,omitempty" jsonschema:"id, name, created_at, updated_at, last_activity_at, stars"`
	Sort       string `json:"sort,omitempty" jsonschema:"asc or desc"`
	PerPage    *int   `json:"per_page,omitempty" jsonschema:"Items per page (1-100, default 50)"`
}

func (s *Server) listProjects(ctx context.Context, _ *mcp.CallToolRequest, input ListProjectsInput) (*mcp.CallToolResult, any, error) {
	query := url.Values{}
	setText(query, "search", input.Search)

	membership := true
	if input.Membership != nil {
		membership = *input.Membership
	}
	query.Set("membership", strconv.FormatBool(membership))

	if input.Owned != nil {
		if *input.Owned {
			query.Set("owned", "true")
		}
	}
	if input.Archived != nil {
		query.Set("archived", strconv.FormatBool(*input.Archived))
	}
	if input.Starred != nil {
		if *input.Starred {
			query.Set("starred", "true")
		}
	}

	setOrder(query, input.OrderBy, input.Sort)

	items, err := s.client.List(ctx, "/projects", query, perPage(input.PerPage))
	if err != nil {
		return nil, nil, err
	}
	return textResult(formatProjectsTable(items))
}

type ListUsersInput struct {
	Search   string `json:"search,omitempty" jsonschema:"Search by name or email"`
	Username string `json:"username,omitempty" jsonschema:"Exact username"`
	Active   *bool  `json:"active,omitempty" jsonschema:"Only active users"`
	PerPage  *int   `json:"per_page,omitempty" jsonschema:"Items per page (1-100, default 50)"`
}

func (s *Server) listUsers(ctx context.Context, _ *mcp.CallToolRequest, input ListUsersInput) (*mcp.CallToolResult, any, error) {
	query := url.Values{}
	setText(query, "search", input.Search)
	setText(query, "username", input.Username)

	if input.Active != nil {
		query.Set("active", strconv.FormatBool(*input.Active))
	}

	items, err := s.client.List(ctx, "/users", query, perPage(input.PerPage))
	if err != nil {
		return nil, nil, err
	}
	return textResult(formatUsersTable(items))
}

type ListIssuesInput struct {
	ProjectID    string `json:"project_id,omitempty" jsonschema:"Project ID or path (or use group_id)"`
	GroupID      *int   `json:"group_id,omitempty" jsonschema:"Group ID (or use project_id)"`
	State        string `json:"state,omitempty" jsonschema:"opened, closed or all"`
	Labels       string `json:"labels,omitempty" jsonschema:"Comma-separated labels"`
	Milestone    string `json:"milestone,omitempty" jsonschema:"Milestone title"`
	AuthorID     *int   `json:"author_id,omitempty" jsonschema:"Author ID"`
	AssigneeID   *int   `json:"assignee_id,omitempty" jsonschema:"Assignee ID"`
	Search       string `json:"search,omitempty" jsonschema:"Search in title/description"`
	CreatedAfter string `json:"created_after,omitempty" jsonschema:"Created after (ISO8601)"`
	UpdatedAfter string `json:"updated_after,omitempty" jsonschema:"Updated after (ISO8601)"`
	OrderBy      string `json:"order_by,omitempty" jsonschema:"created_at, updated_at, priority, due_date, relative_position, label_priority"`
	Sort         string `json:"sort,omitempty" jsonschema:"asc or desc"`
	PerPage      *int   `json:"per_page,omitempty" jsonschema:"Items per page (1-100, default 50)"`
}

func (s *Server) listIssues(ctx context.Context, _ *mcp.CallToolRequest, input ListIssuesInput) (*mcp.CallToolResult, any, error) {
	endpoint := scopedEndpoint("/issues", "/issues", "/groups/%d/issues", input.ProjectID, input.GroupID)

	query := url.Values{}
	setText(query, "state", input.State)
	setText(query, "search", input.Search)
	setText(query, "labels", input.Labels)
	setText(query, "milestone", input.Milestone)
	optionalID(query, "author_id", input.AuthorID)
	optionalID(query, "assignee_id", input.AssigneeID)
	setText(query, "created_after", input.CreatedAfter)
	setText(query, "updated_after", input.UpdatedAfter)
	setOrder(query, input.OrderBy, input.Sort)

	items, err := s.client.List(ctx, endpoint, query, perPage(input.PerPage))
	if err != nil {
		return nil, nil, err
	}
	return textResult(formatIssuesTable(items))
}

type ListMergeRequestsInput struct {
	ProjectID    string `json:"project_id,omitempty" jsonschema:"Project ID or path (or use group_id)"`
	GroupID      *int   `json:"group_id,omitempty" jsonschema:"Group ID (or use project_id)"`
	State        string `json:"state,omitempty" jsonschema:"opened, closed, locked, merged or all"`
	AuthorID     *int   `json:"author_id,omitempty" jsonschema:"Author ID"`
	AssigneeID   *int   `json:"assignee_id,omitempty" jsonschema:"Assignee ID"`
	TargetBranch string `json:"target_branch,omitempty" jsonschema:"Target branch"`
	SourceBranch string `json:"source_branch,omitempty" jsonschema:"Source branch"`
	Milestone    string `json:"milestone,omitempty" jsonschema:"Milestone title or ID"`
	Labels       string `json:"labels,omitempty" jsonschema:"Comma-separated labels"`
	Search       string `json:"search,omitempty" jsonschema:"Search in title/description"`
	OrderBy      string `json:"order_by,omitempty" jsonschema:"created_at, updated_at, title"`
	Sort         string `json:"sort,omitempty" jsonschema:"asc or desc"`
	PerPage      *int   `json:"per_page,omitempty" jsonschema:"Items per page (1-100, default 50)"`
}

func (s *Server) listMergeRequests(ctx context.Context, _ *mcp.CallToolRequest, input ListMergeRequestsInput) (*mcp.CallToolResult, any, error) {
	endpoint := scopedEndpoint("/merge_requests", "/merge_requests", "/groups/%d/merge_requests", input.ProjectID, input.GroupID)

	query := url.Values{}
	setText(query, "state", input.State)
	setText(query, "search", input.Search)
	setText(query, "labels", input.Labels)
	setText(query, "milestone", input.Milestone)
	optionalID(query, "author_id", input.AuthorID)
	optionalID(query, "assignee_id", input.AssigneeID)
	setText(query, "target_branch", input.TargetBranch)
	setText(query, "source_branch", input.SourceBranch)
	setOrder(query, input.OrderBy, input.Sort)

	items, err := s.client.List(ctx, endpoint, query, perPage(input.PerPage))
	if err != nil {
		return nil, nil, err
	}
	return textResult(formatMergeRequestsTable(items))
}

type ListPipelinesInput struct {
	ProjectID    string `json:"project_id" jsonschema:"Project ID or path (e.g. 42 or group/project)"`
	Status       string `json:"status,omitempty" jsonschema:"pending, running, success, failed, canceled, skipped, created, waiting_for_resource, preparing"`
	Ref          string `json:"ref,omitempty" jsonschema:"Branch or tag"`
	SHA          string `json:"sha,omitempty" jsonschema:"Commit SHA"`
	UpdatedAfter string `json:"updated_after,omitempty" jsonschema:"Updated after (ISO8601)"`
	Username     string `json:"username,omitempty" jsonschema:"User who triggered"`
	OrderBy      string `json:"order_by,omitempty" jsonschema:"id, status, ref, updated_at, user_id"`
	Sort         string `json:"sort,omitempty" jsonschema:"asc or desc"`
	PerPage      *int   `json:"per_page,omitempty" jsonschema:"Items per page (1-100, default 50)"`
}

func (s *Server) listPipelines(ctx context.Context, _ *mcp.CallToolRequest, input ListPipelinesInput) (*mcp.CallToolResult, any, error) {
	if err := requireProject(input.ProjectID); err != nil {
		return nil, nil, err
	}

	query := url.Values{}
	setText(query, "status", input.Status)
	setText(query, "ref", input.Ref)
	setText(query, "sha", input.SHA)
	setText(query, "updated_after", input.UpdatedAfter)
	setText(query, "username", input.Username)
	setOrder(query, input.OrderBy, input.Sort)

	endpoint := projectEndpoint(input.ProjectID, "/pipelines")
	items, err := s.client.List(ctx, endpoint, query, perPage(input.PerPage))
	if err != nil {
		return nil, nil, err
	}
	return textResult(formatPipelinesTable(items))
}

type ListBranchesInput struct {
	ProjectID string `json:"project_id" jsonschema:"Project ID or path (e.g. 42 or group/project)"`
	Search    string `json:"search,omitempty" jsonschema:"Search by branch name"`
	PerPage   *int   `json:"per_page,omitempty" jsonschema:"Items per page (1-100, default 50)"`
}

func (s *Server) listBranches(ctx context.Context, _ *mcp.CallToolRequest, input ListBranchesInput) (*mcp.CallToolResult, any, error) {
	if err := requireProject(input.ProjectID); err != nil {
		return nil, nil, err
	}

	query := url.Values{}
	setText(query, "search", input.Search)

	endpoint := projectEndpoint(input.ProjectID, "/repository/branches")
	items, err := s.client.List(ctx, endpoint, query, perPage(input.PerPage))
	if err != nil {
		return nil, nil, err
	}
	return textResult(formatBranchesTable(items))
}

type ListCommitsInput struct {
	ProjectID string `json:"project_id" jsonschema:"Project ID or path (e.g. 42 or group/project)"`
	RefName   string `json:"ref_name,omitempty" jsonschema:"Branch or tag"`
	Since     string `json:"since,omitempty" jsonschema:"Commits since (ISO8601)"`
	Until     string `json:"until,omitempty" jsonschema:"Commits until (ISO8601)"`
	Author    string `json:"author,omitempty" jsonschema:"Author email or name"`
	PerPage   *int   `json:"per_page,omitempty" jsonschema:"Items per page (1-100, default 50)"`
}

func (s *Server) listCommits(ctx context.Context, _ *mcp.CallToolRequest, input ListCommitsInput) (*mcp.CallToolResult, any, error) {
	if err := requireProject(input.ProjectID); err != nil {
		return nil, nil, err
	}

	query := url.Values{}
	setText(query, "ref_name", input.RefName)
	setText(query, "since", input.Since)
	setText(query, "until", input.Until)
	setText(query, "author", input.Author)

	endpoint := projectEndpoint(input.ProjectID, "/repository/commits")
	items, err := s.client.List(ctx, endpoint, query, perPage(input.PerPage))
	if err != nil {
		return nil, nil, err
	}
	return textResult(formatCommitsTable(items))
}

type ListProjectMembersInput struct {
	ProjectID string `json:"project_id,omitempty" jsonschema:"Project ID or path (or use group_id)"`
	GroupID   *int   `json:"group_id,omitempty" jsonschema:"Group ID (or use project_id)"`
	Search    string `json:"search,omitempty" jsonschema:"Search by name/username"`
	PerPage   *int   `json:"per_page,omitempty" jsonschema:"Items per page (1-100, default 50)"`
}

func (s *Server) listProjectMembers(ctx context.Context, _ *mcp.CallToolRequest, input ListProjectMembersInput) (*mcp.CallToolResult, any, error) {
	endpoint := ""
	switch {
	case hasProject(input.ProjectID):
		endpoint = projectEndpoint(input.ProjectID, "/members/all")
	case validID(input.GroupID):
		endpoint = fmt.Sprintf("/groups/%d/members/all", *input.GroupID)
	default:
		return nil, nil, errors.New("informe project_id ou group_id")
	}

	query := url.Values{}
	setText(query, "search", input.Search)

	items, err := s.client.List(ctx, endpoint, query, perPage(input.PerPage))
	if err != nil {
		return nil, nil, err
	}
	return textResult(formatMembersTable(items))
}

type ListTagsInput struct {
	ProjectID string `json:"project_id" jsonschema:"Project ID or path (e.g. 42 or group/project)"`
	OrderBy   string `json:"order_by,omitempty" jsonschema:"name, updated or version"`
	Sort      string `json:"sort,omitempty" jsonschema:"asc or desc"`
	Search    string `json:"search,omitempty" jsonschema:"Search by tag name"`
	PerPage   *int   `json:"per_page,omitempty" jsonschema:"Items per page (1-100, default 50)"`
}

func (s *Server) listTags(ctx context.Context, _ *mcp.CallToolRequest, input ListTagsInput) (*mcp.CallToolResult, any, error) {
	if err := requireProject(input.ProjectID); err != nil {
		return nil, nil, err
	}

	query := url.Values{}
	setOrder(query, input.OrderBy, input.Sort)
	setText(query, "search", input.Search)

	endpoint := projectEndpoint(input.ProjectID, "/repository/tags")
	items, err := s.client.List(ctx, endpoint, query, perPage(input.PerPage))
	if err != nil {
		return nil, nil, err
	}
	return textResult(formatTagsTable(items))
}

type ListJobsInput struct {
	ProjectID  string `json:"project_id" jsonschema:"Project ID or path (e.g. 42 or group/project)"`
	PipelineID *int   `json:"pipeline_id,omitempty" jsonschema:"Pipeline ID; when set, lists only that pipeline's jobs (ignores scope)"`
	Scope      string `json:"scope,omitempty" jsonschema:"Status filter (without pipeline_id): created, pending, running, failed, success, canceled, skipped, manual, waiting_for_resource"`
	PerPage    *int   `json:"per_page,omitempty" jsonschema:"Items per page (1-100, default 50)"`
}

func (s *Server) listJobs(ctx context.Context, _ *mcp.CallToolRequest, input ListJobsInput) (*mcp.CallToolResult, any, error) {
	if err := requireProject(input.ProjectID); err != nil {
		return nil, nil, err
	}

	endpoint, query := jobsRequest(input.ProjectID, input.PipelineID, input.Scope)
	items, err := s.client.List(ctx, endpoint, query, perPage(input.PerPage))
	if err != nil {
		return nil, nil, err
	}
	return textResult(formatJobsTable(items))
}

type ListInput struct {
	Entity       string `json:"entity" jsonschema:"Entity: projects, users, issues, merge_requests, pipelines, branches, commits, members, tags, jobs or tree"`
	ProjectID    string `json:"project_id,omitempty" jsonschema:"Project ID or path (required for pipelines/branches/commits/tags/jobs)"`
	GroupID      *int   `json:"group_id,omitempty" jsonschema:"Group ID (issues/merge_requests/members; alternative to project_id)"`
	Search       string `json:"search,omitempty" jsonschema:"Search term (projects/users/issues/merge_requests/branches/members/tags)"`
	Username     string `json:"username,omitempty" jsonschema:"Exact username (users)"`
	Active       *bool  `json:"active,omitempty" jsonschema:"Only active users (users)"`
	Membership   *bool  `json:"membership,omitempty" jsonschema:"Only projects you're a member of (projects)"`
	Owned        *bool  `json:"owned,omitempty" jsonschema:"Only projects you own (projects)"`
	Archived     *bool  `json:"archived,omitempty" jsonschema:"Include archived (projects)"`
	Starred      *bool  `json:"starred,omitempty" jsonschema:"Only starred projects (projects)"`
	State        string `json:"state,omitempty" jsonschema:"State filter (issues/merge_requests)"`
	Labels       string `json:"labels,omitempty" jsonschema:"Comma-separated labels (issues/merge_requests)"`
	Milestone    string `json:"milestone,omitempty" jsonschema:"Milestone title or ID (issues/merge_requests)"`
	AuthorID     *int   `json:"author_id,omitempty" jsonschema:"Author ID (issues/merge_requests)"`
	AssigneeID   *int   `json:"assignee_id,omitempty" jsonschema:"Assignee ID (issues/merge_requests)"`
	CreatedAfter string `json:"created_after,omitempty" jsonschema:"Created after (ISO8601) (issues)"`
	UpdatedAfter string `json:"updated_after,omitempty" jsonschema:"Updated after (ISO8601) (issues/pipelines)"`
	TargetBranch string `json:"target_branch,omitempty" jsonschema:"Target branch (merge_requests)"`
	SourceBranch string `json:"source_branch,omitempty" jsonschema:"Source branch (merge_requests)"`
	Status       string `json:"status,omitempty" jsonschema:"Pipeline status (pipelines)"`
	Ref          string `json:"ref,omitempty" jsonschema:"Branch or tag (pipelines/tree)"`
	SHA          string `json:"sha,omitempty" jsonschema:"Commit SHA (pipelines)"`
	Path         string `json:"path,omitempty" jsonschema:"Subdirectory path (tree)"`
	Recursive    *bool  `json:"recursive,omitempty" jsonschema:"Recurse into subdirectories (tree)"`
	RefName      string `json:"ref_name,omitempty" jsonschema:"Branch or tag (commits)"`
	Since        string `json:"since,omitempty" jsonschema:"Since (ISO8601) (commits)"`
	Until        string `json:"until,omitempty" jsonschema:"Until (ISO8601) (commits)"`
	Author       string `json:"author,omitempty" jsonschema:"Author email/name (commits)"`
	OrderBy      string `json:"order_by,omitempty" jsonschema:"Order by field"`
	Sort         string `json:"sort,omitempty" jsonschema:"asc or desc"`
	PipelineID   *int   `json:"pipeline_id,omitempty" jsonschema:"Pipeline ID (jobs; lists only that pipeline's jobs)"`
	Scope        string `json:"scope,omitempty" jsonschema:"Job status filter (jobs)"`
	PerPage      *int   `json:"per_page,omitempty" jsonschema:"Items per page (1-100, default 50)"`
}

func (s *Server) list(ctx context.Context, _ *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, any, error) {
	switch strings.ToLower(strings.TrimSpace(input.Entity)) {
	case "project", "projects":
		return s.listProjects(ctx, nil, ListProjectsInput{
			Search: input.Search, Membership: input.Membership, Owned: input.Owned,
			Archived: input.Archived, Starred: input.Starred, OrderBy: input.OrderBy,
			Sort: input.Sort, PerPage: input.PerPage,
		})
	case "user", "users":
		return s.listUsers(ctx, nil, ListUsersInput{
			Search: input.Search, Username: input.Username, Active: input.Active, PerPage: input.PerPage,
		})
	case "issue", "issues":
		return s.listIssues(ctx, nil, ListIssuesInput{
			ProjectID: input.ProjectID, GroupID: input.GroupID, State: input.State,
			Labels: input.Labels, Milestone: input.Milestone, AuthorID: input.AuthorID,
			AssigneeID: input.AssigneeID, Search: input.Search, CreatedAfter: input.CreatedAfter,
			UpdatedAfter: input.UpdatedAfter, OrderBy: input.OrderBy, Sort: input.Sort, PerPage: input.PerPage,
		})
	case "merge_request", "merge_requests", "mr":
		return s.listMergeRequests(ctx, nil, ListMergeRequestsInput{
			ProjectID: input.ProjectID, GroupID: input.GroupID, State: input.State,
			AuthorID: input.AuthorID, AssigneeID: input.AssigneeID, TargetBranch: input.TargetBranch,
			SourceBranch: input.SourceBranch, Milestone: input.Milestone, Labels: input.Labels,
			Search: input.Search, OrderBy: input.OrderBy, Sort: input.Sort, PerPage: input.PerPage,
		})
	case "pipeline", "pipelines":
		return s.listPipelines(ctx, nil, ListPipelinesInput{
			ProjectID: input.ProjectID, Status: input.Status, Ref: input.Ref, SHA: input.SHA,
			UpdatedAfter: input.UpdatedAfter, Username: input.Username, OrderBy: input.OrderBy,
			Sort: input.Sort, PerPage: input.PerPage,
		})
	case "branch", "branches":
		return s.listBranches(ctx, nil, ListBranchesInput{
			ProjectID: input.ProjectID, Search: input.Search, PerPage: input.PerPage,
		})
	case "commit", "commits":
		return s.listCommits(ctx, nil, ListCommitsInput{
			ProjectID: input.ProjectID, RefName: input.RefName, Since: input.Since,
			Until: input.Until, Author: input.Author, PerPage: input.PerPage,
		})
	case "member", "members":
		return s.listProjectMembers(ctx, nil, ListProjectMembersInput{
			ProjectID: input.ProjectID, GroupID: input.GroupID, Search: input.Search, PerPage: input.PerPage,
		})
	case "tag", "tags":
		return s.listTags(ctx, nil, ListTagsInput{
			ProjectID: input.ProjectID, OrderBy: input.OrderBy, Sort: input.Sort, Search: input.Search, PerPage: input.PerPage,
		})
	case "job", "jobs":
		return s.listJobs(ctx, nil, ListJobsInput{
			ProjectID: input.ProjectID, PipelineID: input.PipelineID, Scope: input.Scope, PerPage: input.PerPage,
		})
	case "tree", "trees":
		return s.getTree(ctx, nil, GetTreeInput{
			ProjectID: input.ProjectID, Path: input.Path, Ref: input.Ref, Recursive: input.Recursive,
		})
	default:
		return nil, nil, fmt.Errorf("entidade inválida: %q (use 'projects', 'users', 'issues', 'merge_requests', 'pipelines', 'branches', 'commits', 'members', 'tags', 'jobs' ou 'tree')", input.Entity)
	}
}

type GetItemInput struct {
	Entity    string `json:"entity" jsonschema:"Entity: project, user, issue, merge_request, pipeline, branch, commit, member, tag or job"`
	ID        string `json:"id" jsonschema:"Item ID/ref: numeric id or group/project path for project; numeric id for user/issue/MR/pipeline/member/job; branch/tag name or commit SHA for branch/tag/commit"`
	ProjectID string `json:"project_id,omitempty" jsonschema:"Project ID or path (required for issue/MR/pipeline/branch/commit/tag/job)"`
	GroupID   *int   `json:"group_id,omitempty" jsonschema:"Group ID (members; alternative to project_id)"`
}

func needProjectID(entity string) bool {
	switch entity {
	case "issue", "issues", "merge_request", "merge_requests", "mr", "pipeline", "pipelines",
		"branch", "branches", "commit", "commits", "tag", "tags", "job", "jobs":
		return true
	default:
		return false
	}
}

func itemEndpoint(entity, id, project string, groupID *int) (string, error) {
	switch entity {
	case "project", "projects":
		return fmt.Sprintf("/projects/%s", projectPath(id)), nil
	case "user", "users":
		return fmt.Sprintf("/users/%s", id), nil
	case "issue", "issues":
		return projectEndpoint(project, fmt.Sprintf("/issues/%s", id)), nil
	case "merge_request", "merge_requests", "mr":
		return projectEndpoint(project, fmt.Sprintf("/merge_requests/%s", id)), nil
	case "pipeline", "pipelines":
		return projectEndpoint(project, fmt.Sprintf("/pipelines/%s", id)), nil
	case "branch", "branches":
		return projectEndpoint(project, fmt.Sprintf("/repository/branches/%s", url.PathEscape(id))), nil
	case "commit", "commits":
		return projectEndpoint(project, fmt.Sprintf("/repository/commits/%s", id)), nil
	case "member", "members":
		endpoint := memberEndpoint(project, groupID, id)
		if endpoint == "" {
			return "", errors.New("informe project_id ou group_id")
		}
		return endpoint, nil
	case "tag", "tags":
		return projectEndpoint(project, fmt.Sprintf("/repository/tags/%s", url.PathEscape(id))), nil
	case "job", "jobs":
		return projectEndpoint(project, fmt.Sprintf("/jobs/%s", id)), nil
	default:
		return "", fmt.Errorf("entity inválida: %q (use 'projects','users','issues','merge_requests','pipelines','branches','commits','members','tags' ou 'jobs')", entity)
	}
}

func (s *Server) getItem(ctx context.Context, _ *mcp.CallToolRequest, input GetItemInput) (*mcp.CallToolResult, any, error) {
	entity := strings.ToLower(strings.TrimSpace(input.Entity))
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return nil, nil, errors.New("id é obrigatório")
	}

	if needProjectID(entity) {
		if err := requireProject(input.ProjectID); err != nil {
			return nil, nil, err
		}
	}

	endpoint, err := itemEndpoint(entity, id, input.ProjectID, input.GroupID)
	if err != nil {
		return nil, nil, err
	}

	raw, err := s.client.Get(ctx, endpoint, url.Values{})
	if err != nil {
		return nil, nil, err
	}
	return textResult(formatDetailItem(entity, raw))
}

type GetTreeInput struct {
	ProjectID string `json:"project_id" jsonschema:"Project ID or path (e.g. 42 or group/project)"`
	Path      string `json:"path,omitempty" jsonschema:"Subdirectory path (files under this directory)"`
	Ref       string `json:"ref,omitempty" jsonschema:"Branch/tag/commit (default: default branch)"`
	Recursive *bool  `json:"recursive,omitempty" jsonschema:"Recurse into subdirectories (default: false)"`
}

func (s *Server) getTree(ctx context.Context, _ *mcp.CallToolRequest, input GetTreeInput) (*mcp.CallToolResult, any, error) {
	if err := requireProject(input.ProjectID); err != nil {
		return nil, nil, err
	}

	query := url.Values{}
	setText(query, "path", input.Path)
	setText(query, "ref", input.Ref)

	if input.Recursive != nil {
		if *input.Recursive {
			query.Set("recursive", "true")
		}
	}

	endpoint := projectEndpoint(input.ProjectID, "/repository/tree")
	items, err := s.client.List(ctx, endpoint, query, MAX_TREE_ITEMS)
	if err != nil {
		return nil, nil, err
	}
	return textResult(formatTreeTable(items))
}

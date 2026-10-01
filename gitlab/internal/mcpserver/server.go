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
		Description: "List GitLab resources. Set 'entity' (projects|users|issues|merge_requests|pipelines|branches|commits|members|tags|jobs). Filters vary per entity; pipelines/branches/commits/tags/jobs require project_id.",
	}, s.list)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_item",
		Description: "Get a single GitLab item by id. entity = project|user|issue|merge_request|pipeline|branch|commit|member|tag|job. Many entities require project_id.",
	}, s.getItem)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_file",
		Description: "Get a file's content from a repository (project_id, path). ref = branch/tag/commit, default is the default branch. Files over 200KB are truncated.",
	}, s.getFile)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_tree",
		Description: "List files in a repository tree (project_id). Filters: path, ref, recursive.",
	}, s.getTree)
}

type GetFileInput struct {
	ProjectID int    `json:"project_id" jsonschema:"Project ID (required)"`
	Path      string `json:"path" jsonschema:"File path in the repository (e.g. src/main.go)"`
	Ref       string `json:"ref,omitempty" jsonschema:"Branch/tag/commit (default: default branch)"`
}

func validID(p *int) bool {
	if p == nil {
		return false
	}
	return *p > 0
}

func requireProjectID(p *int) error {
	if validID(p) {
		return nil
	}
	return errors.New("project_id é obrigatório")
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

func scopedEndpoint(fallback, projectPath, groupPath string, projectID, groupID *int) string {
	if validID(projectID) {
		return fmt.Sprintf(projectPath, *projectID)
	}
	if validID(groupID) {
		return fmt.Sprintf(groupPath, *groupID)
	}
	return fallback
}

func memberEndpoint(projectID, groupID *int, id string) string {
	if validID(projectID) {
		return fmt.Sprintf("/projects/%d/members/%s", *projectID, id)
	}
	if validID(groupID) {
		return fmt.Sprintf("/groups/%d/members/%s", *groupID, id)
	}
	return ""
}

func jobsRequest(projectID, pipelineID *int, scope string) (string, url.Values) {
	query := url.Values{}
	endpoint := fmt.Sprintf("/projects/%d/jobs", *projectID)
	if validID(pipelineID) {
		return fmt.Sprintf("/projects/%d/pipelines/%d/jobs", *projectID, *pipelineID), query
	}

	setText(query, "scope", scope)
	return endpoint, query
}

func (s *Server) defaultBranch(ctx context.Context, projectID int) string {
	raw, err := s.client.Get(ctx, fmt.Sprintf("/projects/%d", projectID), url.Values{})
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
	if input.ProjectID <= 0 {
		return nil, nil, errors.New("project_id é obrigatório")
	}

	path := strings.TrimSpace(input.Path)
	if path == "" {
		return nil, nil, errors.New("path é obrigatório")
	}

	ref := strings.TrimSpace(input.Ref)
	if ref == "" {
		ref = s.defaultBranch(ctx, input.ProjectID)
	}

	query := url.Values{}
	query.Set("ref", ref)

	endpoint := fmt.Sprintf("/projects/%d/repository/files/%s", input.ProjectID, url.PathEscape(path))
	raw, err := s.client.Get(ctx, endpoint, query)
	if err != nil {
		return nil, nil, err
	}

	f, ok := raw.(map[string]any)
	if !ok {
		return nil, nil, errors.New("resposta inesperada do GitLab")
	}

	content := fileContent(f)
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
		b.WriteString("\n\n⚠️ Arquivo maior que 200KB: conteúdo truncado. Use gitlab_get_file com um ref específico se precisar de outra parte (ou rode localmente).")
	}

	return textResult(strings.TrimSpace(b.String()))
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
	ProjectID *int   `json:"project_id,omitempty" jsonschema:"Restrict to a project (required for some scopes, e.g. commits)"`
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
	optionalID(query, "project_id", input.ProjectID)
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
	ProjectID    *int   `json:"project_id,omitempty" jsonschema:"Project ID (or use group_id)"`
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
	endpoint := scopedEndpoint("/issues", "/projects/%d/issues", "/groups/%d/issues", input.ProjectID, input.GroupID)

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
	ProjectID    *int   `json:"project_id,omitempty" jsonschema:"Project ID (or use group_id)"`
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
	endpoint := scopedEndpoint("/merge_requests", "/projects/%d/merge_requests", "/groups/%d/merge_requests", input.ProjectID, input.GroupID)

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
	ProjectID    *int   `json:"project_id" jsonschema:"Project ID (required)"`
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
	if err := requireProjectID(input.ProjectID); err != nil {
		return nil, nil, err
	}

	query := url.Values{}
	setText(query, "status", input.Status)
	setText(query, "ref", input.Ref)
	setText(query, "sha", input.SHA)
	setText(query, "updated_after", input.UpdatedAfter)
	setText(query, "username", input.Username)
	setOrder(query, input.OrderBy, input.Sort)

	endpoint := fmt.Sprintf("/projects/%d/pipelines", *input.ProjectID)
	items, err := s.client.List(ctx, endpoint, query, perPage(input.PerPage))
	if err != nil {
		return nil, nil, err
	}
	return textResult(formatPipelinesTable(items))
}

type ListBranchesInput struct {
	ProjectID *int   `json:"project_id" jsonschema:"Project ID (required)"`
	Search    string `json:"search,omitempty" jsonschema:"Search by branch name"`
	PerPage   *int   `json:"per_page,omitempty" jsonschema:"Items per page (1-100, default 50)"`
}

func (s *Server) listBranches(ctx context.Context, _ *mcp.CallToolRequest, input ListBranchesInput) (*mcp.CallToolResult, any, error) {
	if err := requireProjectID(input.ProjectID); err != nil {
		return nil, nil, err
	}

	query := url.Values{}
	setText(query, "search", input.Search)

	endpoint := fmt.Sprintf("/projects/%d/repository/branches", *input.ProjectID)
	items, err := s.client.List(ctx, endpoint, query, perPage(input.PerPage))
	if err != nil {
		return nil, nil, err
	}
	return textResult(formatBranchesTable(items))
}

type ListCommitsInput struct {
	ProjectID *int   `json:"project_id" jsonschema:"Project ID (required)"`
	RefName   string `json:"ref_name,omitempty" jsonschema:"Branch or tag"`
	Since     string `json:"since,omitempty" jsonschema:"Commits since (ISO8601)"`
	Until     string `json:"until,omitempty" jsonschema:"Commits until (ISO8601)"`
	Author    string `json:"author,omitempty" jsonschema:"Author email or name"`
	PerPage   *int   `json:"per_page,omitempty" jsonschema:"Items per page (1-100, default 50)"`
}

func (s *Server) listCommits(ctx context.Context, _ *mcp.CallToolRequest, input ListCommitsInput) (*mcp.CallToolResult, any, error) {
	if err := requireProjectID(input.ProjectID); err != nil {
		return nil, nil, err
	}

	query := url.Values{}
	setText(query, "ref_name", input.RefName)
	setText(query, "since", input.Since)
	setText(query, "until", input.Until)
	setText(query, "author", input.Author)

	endpoint := fmt.Sprintf("/projects/%d/repository/commits", *input.ProjectID)
	items, err := s.client.List(ctx, endpoint, query, perPage(input.PerPage))
	if err != nil {
		return nil, nil, err
	}
	return textResult(formatCommitsTable(items))
}

type ListProjectMembersInput struct {
	ProjectID *int   `json:"project_id,omitempty" jsonschema:"Project ID (or use group_id)"`
	GroupID   *int   `json:"group_id,omitempty" jsonschema:"Group ID (or use project_id)"`
	Search    string `json:"search,omitempty" jsonschema:"Search by name/username"`
	PerPage   *int   `json:"per_page,omitempty" jsonschema:"Items per page (1-100, default 50)"`
}

func (s *Server) listProjectMembers(ctx context.Context, _ *mcp.CallToolRequest, input ListProjectMembersInput) (*mcp.CallToolResult, any, error) {
	endpoint := ""
	switch {
	case validID(input.ProjectID):
		endpoint = fmt.Sprintf("/projects/%d/members/all", *input.ProjectID)
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
	ProjectID *int   `json:"project_id" jsonschema:"Project ID (required)"`
	OrderBy   string `json:"order_by,omitempty" jsonschema:"name, updated or version"`
	Sort      string `json:"sort,omitempty" jsonschema:"asc or desc"`
	Search    string `json:"search,omitempty" jsonschema:"Search by tag name"`
	PerPage   *int   `json:"per_page,omitempty" jsonschema:"Items per page (1-100, default 50)"`
}

func (s *Server) listTags(ctx context.Context, _ *mcp.CallToolRequest, input ListTagsInput) (*mcp.CallToolResult, any, error) {
	if err := requireProjectID(input.ProjectID); err != nil {
		return nil, nil, err
	}

	query := url.Values{}
	setOrder(query, input.OrderBy, input.Sort)
	setText(query, "search", input.Search)

	endpoint := fmt.Sprintf("/projects/%d/repository/tags", *input.ProjectID)
	items, err := s.client.List(ctx, endpoint, query, perPage(input.PerPage))
	if err != nil {
		return nil, nil, err
	}
	return textResult(formatTagsTable(items))
}

type ListJobsInput struct {
	ProjectID  *int   `json:"project_id" jsonschema:"Project ID (required)"`
	PipelineID *int   `json:"pipeline_id,omitempty" jsonschema:"Pipeline ID; when set, lists only that pipeline's jobs (ignores scope)"`
	Scope      string `json:"scope,omitempty" jsonschema:"Status filter (without pipeline_id): created, pending, running, failed, success, canceled, skipped, manual, waiting_for_resource"`
	PerPage    *int   `json:"per_page,omitempty" jsonschema:"Items per page (1-100, default 50)"`
}

func (s *Server) listJobs(ctx context.Context, _ *mcp.CallToolRequest, input ListJobsInput) (*mcp.CallToolResult, any, error) {
	if err := requireProjectID(input.ProjectID); err != nil {
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
	Entity       string `json:"entity" jsonschema:"Entity: projects, users, issues, merge_requests, pipelines, branches, commits, members, tags or jobs"`
	ProjectID    *int   `json:"project_id,omitempty" jsonschema:"Project ID (required for pipelines/branches/commits/tags/jobs)"`
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
	Ref          string `json:"ref,omitempty" jsonschema:"Branch or tag (pipelines)"`
	SHA          string `json:"sha,omitempty" jsonschema:"Commit SHA (pipelines)"`
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
	default:
		return nil, nil, fmt.Errorf("entidade inválida: %q (use 'projects', 'users', 'issues', 'merge_requests', 'pipelines', 'branches', 'commits', 'members', 'tags' ou 'jobs')", input.Entity)
	}
}

type GetItemInput struct {
	Entity    string `json:"entity" jsonschema:"Entity: project, user, issue, merge_request, pipeline, branch, commit, member, tag or job"`
	ID        string `json:"id" jsonschema:"Item ID/ref: numeric id for project/user/issue/MR/pipeline/member/job; branch/tag name or commit SHA for branch/tag/commit"`
	ProjectID *int   `json:"project_id,omitempty" jsonschema:"Project ID (required for issue/MR/pipeline/branch/commit/tag/job)"`
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

func itemEndpoint(entity, id string, projectID, groupID *int) (string, error) {
	switch entity {
	case "project", "projects":
		return fmt.Sprintf("/projects/%s", id), nil
	case "user", "users":
		return fmt.Sprintf("/users/%s", id), nil
	case "issue", "issues":
		return fmt.Sprintf("/projects/%d/issues/%s", *projectID, id), nil
	case "merge_request", "merge_requests", "mr":
		return fmt.Sprintf("/projects/%d/merge_requests/%s", *projectID, id), nil
	case "pipeline", "pipelines":
		return fmt.Sprintf("/projects/%d/pipelines/%s", *projectID, id), nil
	case "branch", "branches":
		return fmt.Sprintf("/projects/%d/repository/branches/%s", *projectID, url.PathEscape(id)), nil
	case "commit", "commits":
		return fmt.Sprintf("/projects/%d/repository/commits/%s", *projectID, id), nil
	case "member", "members":
		endpoint := memberEndpoint(projectID, groupID, id)
		if endpoint == "" {
			return "", errors.New("informe project_id ou group_id")
		}
		return endpoint, nil
	case "tag", "tags":
		return fmt.Sprintf("/projects/%d/repository/tags/%s", *projectID, url.PathEscape(id)), nil
	case "job", "jobs":
		return fmt.Sprintf("/projects/%d/jobs/%s", *projectID, id), nil
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
		if err := requireProjectID(input.ProjectID); err != nil {
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
	ProjectID *int   `json:"project_id" jsonschema:"Project ID (required)"`
	Path      string `json:"path,omitempty" jsonschema:"Subdirectory path (files under this directory)"`
	Ref       string `json:"ref,omitempty" jsonschema:"Branch/tag/commit (default: default branch)"`
	Recursive *bool  `json:"recursive,omitempty" jsonschema:"Recurse into subdirectories (default: false)"`
}

func (s *Server) getTree(ctx context.Context, _ *mcp.CallToolRequest, input GetTreeInput) (*mcp.CallToolResult, any, error) {
	if err := requireProjectID(input.ProjectID); err != nil {
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

	endpoint := fmt.Sprintf("/projects/%d/repository/tree", *input.ProjectID)
	items, err := s.client.List(ctx, endpoint, query, MAX_TREE_ITEMS)
	if err != nil {
		return nil, nil, err
	}
	return textResult(formatTreeTable(items))
}

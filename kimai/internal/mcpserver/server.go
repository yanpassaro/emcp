package mcpserver

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"ntdsk.com/kimai/internal/kimai"
)

const (
	KIMAI_DATE_FORMAT  = "2006-01-02T15:04:05"
	TAGS_PAGE          = "1"
	TAGS_LIMIT         = "1000"
	NOT_RUNNING_NOTICE = "Nenhuma marcação de tempo em andamento."
	SCHEDULE_MISSING   = "🗓️ **Horário fixo:** nenhum configurado (defina KIMAI_SCHEDULE)."
)

type MCPServer struct {
	server   *mcp.Server
	client   *kimai.Client
	schedule *Schedule
}

func NewMCPServer(kimaiClient *kimai.Client, schedule *Schedule) *MCPServer {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "kimai",
		Version: "1.0.0",
	}, nil)

	m := &MCPServer{
		server:   s,
		client:   kimaiClient,
		schedule: schedule,
	}

	m.registerTools()
	return m
}

func (m *MCPServer) Server() *mcp.Server {
	return m.server
}

func (m *MCPServer) Run(ctx context.Context, transport mcp.Transport) error {
	return m.server.Run(ctx, transport)
}

func (m *MCPServer) registerTools() {
	m.registerStartTimeEntryTool()
	m.registerGetTimeEntriesTool()
	m.registerTimeEntryTool()
	m.registerListTool()
	m.registerGetUserInfoTool()
}

func toolHandler[In any](fn func(context.Context, In) (string, error)) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input In) (*mcp.CallToolResult, any, error) {
		text, err := fn(ctx, input)
		if err != nil {
			return nil, nil, err
		}
		return textResult(text)
	}
}

func normalizeKimaiDate(value string) string {
	if value == "" {
		return ""
	}

	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02",
	}

	for _, layout := range layouts {
		t, err := time.Parse(layout, value)
		if err == nil {
			return t.Format(KIMAI_DATE_FORMAT)
		}
	}

	return value
}

func firstDayOfMonth(t time.Time) string {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location()).Format(KIMAI_DATE_FORMAT)
}

func joinStrings(nums []int) []string {
	result := make([]string, len(nums))
	for i, n := range nums {
		result[i] = strconv.Itoa(n)
	}
	return result
}

func tagID(name string) int {
	id, err := strconv.Atoi(strings.TrimPrefix(strings.SplitN(name, " ", 2)[0], "#"))
	if err != nil {
		return 0
	}
	return id
}

func sortedTagNames(names []string) []string {
	sort.Slice(names, func(i, j int) bool { return tagID(names[i]) < tagID(names[j]) })
	return names
}

func (m *MCPServer) validTagNames(ctx context.Context) (map[int]string, error) {
	raw, err := m.client.GetJSON(ctx, "/api/tags", url.Values{
		"page":  {TAGS_PAGE},
		"limit": {TAGS_LIMIT},
	})
	if err != nil {
		return nil, err
	}

	items, ok := raw.([]any)
	if !ok {
		return nil, nil
	}

	valid := map[int]string{}
	for _, it := range items {
		t, ok := it.(map[string]any)
		if !ok {
			continue
		}
		id := displayInt(t["id"])
		if id > 0 {
			valid[id] = displayString(t["name"])
		}
	}

	return valid, nil
}

func (m *MCPServer) missingTags(valid map[int]string, ids []int) []string {
	missing := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := valid[id]; !ok {
			missing = append(missing, strconv.Itoa(id))
		}
	}
	return missing
}

func (m *MCPServer) validateTags(ctx context.Context, ids []int) error {
	if len(ids) == 0 {
		return nil
	}

	valid, err := m.validTagNames(ctx)
	if err != nil {
		return err
	}

	if len(valid) == 0 {
		return nil
	}

	missing := m.missingTags(valid, ids)
	if len(missing) == 0 {
		return nil
	}

	names := make([]string, 0, len(valid))
	for id, name := range valid {
		names = append(names, fmt.Sprintf("#%d %s", id, name))
	}

	return fmt.Errorf("tag(s) %s não existe(m) no Kimai; consulte kimai_list (entity=tags) para ver as tags válidas (%s)",
		strings.Join(missing, ", "), strings.Join(sortedTagNames(names), ", "))
}

func timesheetURL(id int) string {
	return fmt.Sprintf("/api/timesheets/%d", id)
}

type StartTimeEntryInput struct {
	ProjectID   int    `json:"project_id" jsonschema:"ID of the project"`
	ActivityID  int    `json:"activity_id" jsonschema:"ID of the activity"`
	Description string `json:"description,omitempty" jsonschema:"Description for the time entry"`
	Tags        []int  `json:"tags,omitempty" jsonschema:"Array of tag IDs"`
	Begin       string `json:"begin,omitempty" jsonschema:"Start time (format YYYY-MM-DDTHH:MM:SS, no timezone; default: now)"`
	End         string `json:"end,omitempty" jsonschema:"End time (format YYYY-MM-DDTHH:MM:SS, no timezone). If set, creates the entry already completed."`
}

func (m *MCPServer) registerStartTimeEntryTool() {
	mcp.AddTool(m.server, &mcp.Tool{
		Name:        "kimai_start_time_entry",
		Description: "Start a time entry. Omit 'end' to leave it running; set 'end' to create it already completed in one call.",
	}, toolHandler(func(ctx context.Context, input StartTimeEntryInput) (string, error) {
		begin := time.Now().Format(KIMAI_DATE_FORMAT)
		if input.Begin != "" {
			begin = normalizeKimaiDate(input.Begin)
		}

		body := map[string]any{
			"project":  input.ProjectID,
			"activity": input.ActivityID,
			"begin":    begin,
		}

		if input.End != "" {
			body["end"] = normalizeKimaiDate(input.End)
		}
		if input.Description != "" {
			body["description"] = input.Description
		}

		if err := m.validateTags(ctx, input.Tags); err != nil {
			return "", err
		}
		if len(input.Tags) > 0 {
			body["tags"] = input.Tags
		}

		result, err := m.client.PostJSON(ctx, "/api/timesheets", nil, body)
		if err != nil {
			return "", err
		}
		return formatTimeEntry(result), nil
	}))
}

type TimeEntryInput struct {
	Action       string   `json:"action" jsonschema:"Action: 'stop', 'update' or 'delete'"`
	ID           int      `json:"id,omitempty" jsonschema:"Time entry ID (required for stop/update/delete)"`
	End          string   `json:"end,omitempty" jsonschema:"End time, format YYYY-MM-DDTHH:MM:SS (new end for update; default: now for stop)"`
	Begin        string   `json:"begin,omitempty" jsonschema:"New begin time, format YYYY-MM-DDTHH:MM:SS (update)"`
	Description  string   `json:"description,omitempty" jsonschema:"New description (update)"`
	Project      int      `json:"project,omitempty" jsonschema:"New project ID (update)"`
	Activity     int      `json:"activity,omitempty" jsonschema:"New activity ID (update)"`
	Tags         []int    `json:"tags,omitempty" jsonschema:"New tag IDs (update)"`
	Billable     *bool    `json:"billable,omitempty" jsonschema:"New billable status (update)"`
	Exported     *bool    `json:"exported,omitempty" jsonschema:"New exported status (update)"`
	FixedRate    *float64 `json:"fixed_rate,omitempty" jsonschema:"New fixed rate (update)"`
	InternalRate *float64 `json:"internal_rate,omitempty" jsonschema:"New internal rate (update)"`
	HourlyRate   *float64 `json:"hourly_rate,omitempty" jsonschema:"New hourly rate (update)"`
}

func (m *MCPServer) stopTimeEntry(ctx context.Context, input TimeEntryInput) (string, error) {
	if input.ID <= 0 {
		return "", fmt.Errorf("id é obrigatório para stop")
	}

	end := time.Now().Format(KIMAI_DATE_FORMAT)
	if input.End != "" {
		end = normalizeKimaiDate(input.End)
	}

	body := map[string]any{"end": end}
	result, err := m.client.PatchJSON(ctx, fmt.Sprintf("%s/stop", timesheetURL(input.ID)), nil, body)
	if err != nil {
		return "", err
	}
	return formatTimeEntry(result), nil
}

func (m *MCPServer) updateTimeEntry(ctx context.Context, input TimeEntryInput) (string, error) {
	if input.ID <= 0 {
		return "", fmt.Errorf("id é obrigatório para update")
	}

	body := map[string]any{}
	if input.Begin != "" {
		body["begin"] = normalizeKimaiDate(input.Begin)
	}
	if input.End != "" {
		body["end"] = normalizeKimaiDate(input.End)
	}
	if input.Description != "" {
		body["description"] = input.Description
	}
	if input.Project > 0 {
		body["project"] = input.Project
	}
	if input.Activity > 0 {
		body["activity"] = input.Activity
	}

	if err := m.validateTags(ctx, input.Tags); err != nil {
		return "", err
	}
	if len(input.Tags) > 0 {
		body["tags"] = input.Tags
	}

	if input.Billable != nil {
		body["billable"] = *input.Billable
	}
	if input.Exported != nil {
		body["exported"] = *input.Exported
	}
	if input.FixedRate != nil {
		body["fixedRate"] = *input.FixedRate
	}
	if input.InternalRate != nil {
		body["internalRate"] = *input.InternalRate
	}
	if input.HourlyRate != nil {
		body["hourlyRate"] = *input.HourlyRate
	}

	result, err := m.client.PatchJSON(ctx, timesheetURL(input.ID), nil, body)
	if err != nil {
		return "", err
	}
	return formatTimeEntry(result), nil
}

func (m *MCPServer) deleteTimeEntry(ctx context.Context, input TimeEntryInput) (string, error) {
	if input.ID <= 0 {
		return "", fmt.Errorf("id é obrigatório para delete")
	}

	if err := m.client.DeleteJSON(ctx, timesheetURL(input.ID), nil); err != nil {
		return "", err
	}
	return fmt.Sprintf("✅ Marcação de tempo #%d excluída com sucesso.", input.ID), nil
}

func (m *MCPServer) timeEntryAction(ctx context.Context, input TimeEntryInput) (string, error) {
	switch strings.ToLower(strings.TrimSpace(input.Action)) {
	case "stop":
		return m.stopTimeEntry(ctx, input)
	case "update":
		return m.updateTimeEntry(ctx, input)
	case "delete":
		return m.deleteTimeEntry(ctx, input)
	default:
		return "", fmt.Errorf("action deve ser 'stop', 'update' ou 'delete'")
	}
}

func (m *MCPServer) registerTimeEntryTool() {
	mcp.AddTool(m.server, &mcp.Tool{
		Name:        "kimai_time_entry",
		Description: "Change a time entry. 'stop' encerra um lançamento em andamento; 'update' edita campos; 'delete' exclui. Sempre informe o 'id'.",
	}, toolHandler(func(ctx context.Context, input TimeEntryInput) (string, error) {
		return m.timeEntryAction(ctx, input)
	}))
}

func (m *MCPServer) newestRunningEntry(ctx context.Context) (map[string]any, error) {
	items, _, err := m.client.ListJSON(ctx, "/api/timesheets", nil, false)
	if err != nil {
		return nil, err
	}

	for _, item := range items {
		te := asMap(item)
		if te == nil {
			continue
		}
		if e, ok := te["end"]; ok {
			if e != nil {
				continue
			}
		}
		return te, nil
	}

	return nil, nil
}

type GetTimeEntriesInput struct {
	ID       *int   `json:"id,omitempty" jsonschema:"Time entry ID; fetch a single entry instead of listing"`
	Running  *bool  `json:"running,omitempty" jsonschema:"Return only the currently running entry"`
	Begin    string `json:"begin,omitempty" jsonschema:"Filter by begin date (format YYYY-MM-DDTHH:MM:SS, no timezone; default: first day of current month)"`
	End      string `json:"end,omitempty" jsonschema:"Filter by end date (format YYYY-MM-DDTHH:MM:SS, no timezone)"`
	Project  int    `json:"project,omitempty" jsonschema:"Filter by project ID"`
	Activity int    `json:"activity,omitempty" jsonschema:"Filter by activity ID"`
	Customer int    `json:"customer,omitempty" jsonschema:"Filter by customer ID"`
	User     int    `json:"user,omitempty" jsonschema:"Filter by user ID"`
	Tags     []int  `json:"tags,omitempty" jsonschema:"Filter by tag IDs"`
	Billable *bool  `json:"billable,omitempty" jsonschema:"Filter by billable status"`
	Exported *bool  `json:"exported,omitempty" jsonschema:"Filter by exported status"`
	OrderBy  string `json:"order_by,omitempty" jsonschema:"Order by field (begin, end, duration)"`
	OrderDir string `json:"order_dir,omitempty" jsonschema:"Order direction (ASC or DESC)"`
	Page     int    `json:"page,omitempty" jsonschema:"Page number"`
	Limit    int    `json:"limit,omitempty" jsonschema:"Limit per page"`
}

func setIntParam(params url.Values, key string, value int) {
	if value <= 0 {
		return
	}
	params.Set(key, strconv.Itoa(value))
}

func setBoolParam(params url.Values, key string, value *bool) {
	if value == nil {
		return
	}
	params.Set(key, strconv.FormatBool(*value))
}

func setTextParam(params url.Values, key, value string) {
	if value == "" {
		return
	}
	params.Set(key, value)
}

func (m *MCPServer) timeEntryFilters(input GetTimeEntriesInput) url.Values {
	params := url.Values{}
	setTextParam(params, "begin", firstDayOfMonth(time.Now()))
	if input.Begin != "" {
		params.Set("begin", normalizeKimaiDate(input.Begin))
	}

	setTextParam(params, "end", normalizeKimaiDate(input.End))
	setIntParam(params, "project", input.Project)
	setIntParam(params, "activity", input.Activity)
	setIntParam(params, "customer", input.Customer)
	setIntParam(params, "user", input.User)
	setBoolParam(params, "billable", input.Billable)
	setBoolParam(params, "exported", input.Exported)
	setTextParam(params, "orderBy", input.OrderBy)
	setTextParam(params, "orderDir", input.OrderDir)
	setIntParam(params, "page", input.Page)
	setIntParam(params, "limit", input.Limit)

	if len(input.Tags) > 0 {
		params.Set("tags", strings.Join(joinStrings(input.Tags), ","))
	}

	return params
}

func isPaged(page, limit int) bool {
	if page > 0 {
		return true
	}
	return limit > 0
}

func (m *MCPServer) runningTimeEntry(ctx context.Context) (string, error) {
	result, err := m.client.GetJSON(ctx, "/api/timesheets/running", nil)
	if err == nil {
		return formatTimeEntry(result), nil
	}

	if !strings.Contains(err.Error(), "404") {
		return "", err
	}

	entry, ferr := m.newestRunningEntry(ctx)
	if ferr != nil {
		return "", ferr
	}

	if entry == nil {
		return NOT_RUNNING_NOTICE, nil
	}

	return formatTimeEntry(entry), nil
}

func (m *MCPServer) getTimeEntries(ctx context.Context, input GetTimeEntriesInput) (string, error) {
	if input.ID != nil {
		result, err := m.client.GetJSON(ctx, timesheetURL(*input.ID), nil)
		if err != nil {
			return "", err
		}
		return formatTimeEntry(result), nil
	}

	if input.Running != nil {
		if *input.Running {
			return m.runningTimeEntry(ctx)
		}
	}

	params := m.timeEntryFilters(input)
	items, meta, err := m.client.ListJSON(ctx, "/api/timesheets", params, isPaged(input.Page, input.Limit))
	if err != nil {
		return "", err
	}
	return formatTimeEntriesTable(items, meta), nil
}

func (m *MCPServer) registerGetTimeEntriesTool() {
	mcp.AddTool(m.server, &mcp.Tool{
		Name:        "kimai_get_time_entries",
		Description: "Get time entries. List with optional filters (default: from the first day of the current month), fetch a single entry by 'id', or the currently running one with 'running=true'.",
	}, toolHandler(func(ctx context.Context, input GetTimeEntriesInput) (string, error) {
		return m.getTimeEntries(ctx, input)
	}))
}

type ListInput struct {
	Entity    string `json:"entity" jsonschema:"Entity type: projects, customers, activities or tags"`
	ProjectID int    `json:"project_id,omitempty" jsonschema:"Project ID (activities: list that project's activities, including globals)"`
	Customer  int    `json:"customer,omitempty" jsonschema:"Filter by customer ID (projects)"`
	Visible   *bool  `json:"visible,omitempty" jsonschema:"Filter by visible status (projects)"`
	OrderBy   string `json:"order_by,omitempty" jsonschema:"Order by field"`
	OrderDir  string `json:"order_dir,omitempty" jsonschema:"Order direction (ASC or DESC)"`
	Page      int    `json:"page,omitempty" jsonschema:"Page number"`
	Limit     int    `json:"limit,omitempty" jsonschema:"Limit per page"`
}

func (m *MCPServer) fetchCustomerNames(ctx context.Context) (map[int]string, error) {
	items, _, err := m.client.ListJSON(ctx, "/api/customers", url.Values{
		"page":  {TAGS_PAGE},
		"limit": {TAGS_LIMIT},
	}, false)
	if err != nil {
		return nil, err
	}

	names := make(map[int]string, len(items))
	for _, it := range items {
		c, ok := it.(map[string]any)
		if !ok {
			continue
		}
		id := displayInt(c["id"])
		if id <= 0 {
			continue
		}
		n := stringOrEmpty(c["name"])
		if n != "" {
			names[id] = n
		}
	}

	return names, nil
}

func (m *MCPServer) projectActivities(ctx context.Context, projectID int, params url.Values, paginated bool) (string, error) {
	endpoint := fmt.Sprintf("/api/projects/%d/activities", projectID)
	items, meta, err := m.client.ListJSON(ctx, endpoint, params, paginated)
	if err == nil {
		return formatActivitiesTable(items, meta), nil
	}

	fallback, _, ferr := m.client.ListJSON(ctx, "/api/activities", url.Values{}, false)
	if ferr != nil {
		return "", err
	}

	filtered := activitiesForProject(fallback, projectID)
	if len(filtered) == 0 {
		return "Nenhuma atividade vinculada a este projeto (nem global).", nil
	}

	return formatActivitiesTable(filtered, kimai.PaginationMeta{}), nil
}

func (m *MCPServer) list(ctx context.Context, input ListInput) (string, error) {
	params := url.Values{}
	setIntParam(params, "page", input.Page)
	setIntParam(params, "limit", input.Limit)
	paginated := isPaged(input.Page, input.Limit)

	switch strings.ToLower(strings.TrimSpace(input.Entity)) {
	case "project", "projects":
		setIntParam(params, "customer", input.Customer)
		setBoolParam(params, "visible", input.Visible)
		setTextParam(params, "orderBy", input.OrderBy)
		setTextParam(params, "orderDir", input.OrderDir)

		items, meta, err := m.client.ListJSON(ctx, "/api/projects", params, paginated)
		if err != nil {
			return "", err
		}

		names, err := m.fetchCustomerNames(ctx)
		if err != nil {
			names = nil
		}

		return formatProjectsTable(items, meta, names), nil
	case "customer", "customers":
		items, meta, err := m.client.ListJSON(ctx, "/api/customers", params, paginated)
		if err != nil {
			return "", err
		}
		return formatCustomersTable(items, meta), nil
	case "activity", "activities":
		if input.ProjectID > 0 {
			return m.projectActivities(ctx, input.ProjectID, params, paginated)
		}

		items, meta, err := m.client.ListJSON(ctx, "/api/activities", params, paginated)
		if err != nil {
			return "", err
		}
		return formatActivitiesTable(items, meta), nil
	case "tag", "tags":
		result, err := m.client.GetJSON(ctx, "/api/tags", params)
		if err != nil {
			return "", err
		}
		return formatTags(result), nil
	default:
		return "", fmt.Errorf("entidade inválida: %q (use 'projects', 'customers', 'activities' ou 'tags')", input.Entity)
	}
}

func (m *MCPServer) registerListTool() {
	mcp.AddTool(m.server, &mcp.Tool{
		Name:        "kimai_list",
		Description: "List Kimai entities: projects, customers, activities or tags, with optional filters. Use 'entity' plus optional filters (project_id for activities, customer/visible/order for projects, page/limit for paging).",
	}, toolHandler(func(ctx context.Context, input ListInput) (string, error) {
		return m.list(ctx, input)
	}))
}

func activitiesForProject(data any, projectID int) []any {
	items, ok := data.([]any)
	if !ok {
		return nil
	}

	out := make([]any, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}

		p, ok := m["project"].(float64)
		if !ok {
			out = append(out, item)
			continue
		}

		if int(p) == projectID {
			out = append(out, item)
		}
	}

	return out
}

func (m *MCPServer) registerGetUserInfoTool() {
	mcp.AddTool(m.server, &mcp.Tool{
		Name:        "kimai_get_user_info",
		Description: "Get the current authenticated user plus the time/schedule view: current time, today's schedule with the active block highlighted, fixed weekly schedule and month calendar.",
	}, toolHandler(func(ctx context.Context, _ struct{}) (string, error) {
		result, err := m.client.GetJSON(ctx, "/api/users/me", nil)
		if err != nil {
			return "", err
		}

		text := formatUser(result)
		if m.schedule == nil {
			return fmt.Sprintf("%s\n\n%s", text, SCHEDULE_MISSING), nil
		}

		return fmt.Sprintf("%s\n\n%s", text, formatCurrentTimeView(time.Now(), m.schedule)), nil
	}))
}

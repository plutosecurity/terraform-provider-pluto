# Block the GitHub MCP server. Import a policy built in the dashboard to get a full definition to start from.
resource "pluto_policy" "github_mcp" {
  definition = jsonencode({
    type    = "supply-chain"
    name    = "Block the GitHub MCP"
    enabled = true
    conditions = {
      kind = "group", id = "root", conjunction = "and"
      children = [{
        kind   = "condition", id = "c-mcp", field = "mcp:name", operator = "is"
        values = [{ id = "name:github", label = "github", kind = "mcp_server", resolution = "typed", scope = null }]
      }]
    }
    actions = [{ id = "a-outcome", kind = "block", settings = {} }]
  })
}

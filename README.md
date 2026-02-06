# Webex Go MCP Server

A Go-based STDIO [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server that exposes Cisco Webex APIs as tools. This allows LLMs (like Claude) to interact with Webex -- sending messages, managing rooms, scheduling meetings, downloading transcripts, and more.

## Features

**33 MCP tools** across 7 Webex API resource categories:

| Category | Tools | Operations |
|---|---|---|
| **Messages** | 5 | List, create, send attachment, get, delete messages |
| **Rooms** | 5 | List, create, get, update, delete rooms/spaces |
| **Teams** | 4 | List, create, get, update teams |
| **Memberships** | 4 | List, create, update, delete room memberships |
| **Meetings** | 5 | List, create, get, update, delete scheduled meetings |
| **Transcripts** | 5 | List transcripts, download content, list/get/update snippets |
| **Webhooks** | 5 | List, create, get, update, delete webhooks |

## Prerequisites

- Go 1.23 or later
- A [Webex access token](https://developer.webex.com/docs/getting-your-personal-access-token)

## Build

```bash
go build -o webex-go-mcp .
```

## Configuration

Configuration is loaded via environment variables and/or CLI flags. CLI flags take precedence over environment variables.

| Env Variable | CLI Flag | Required | Default | Description |
|---|---|---|---|---|
| `WEBEX_ACCESS_TOKEN` | `--access-token` | Yes | - | Webex API bearer token |
| `WEBEX_BASE_URL` | `--base-url` | No | `https://webexapis.com/v1` | Webex API base URL |
| `WEBEX_TIMEOUT` | `--timeout` | No | `30s` | HTTP request timeout |
| `WEBEX_INCLUDE_TOOLS` | `--include` | No | - | Comma-separated list of tools to include (only these are registered) |
| `WEBEX_EXCLUDE_TOOLS` | `--exclude` | No | - | Comma-separated list of tools to exclude (all others are registered) |
| `WEBEX_MINIMAL` | `--minimal` | No | `false` | Enable minimal tool set (messages, rooms, teams, meetings, transcripts) |
| `WEBEX_READONLY_MINIMAL` | `--readonly-minimal` | No | `false` | Enable readonly minimal tool set (read-only operations only) |

### Tool Filtering

You can control which tools are exposed using `--include` or `--exclude`. Tools are specified in `category:action` format, where `category` maps to the Webex API resource (e.g. `messages`, `rooms`, `meetings`) and `action` is the operation (e.g. `list`, `create`, `get`, `delete`).

Both singular and plural category forms are accepted (`message:list` and `messages:list` both work).

The `category:action` shorthand maps to the full tool name `webex_{category}_{action}`. For example, `messages:list` maps to `webex_messages_list`.

**Rules:**
- If `--include` is set, only the specified tools are registered.
- If `--exclude` is set, all tools except the specified ones are registered.
- If both are set, `--include` takes priority and `--exclude` is ignored.
- If neither is set, all 33 tools are registered (default).

**Available categories and actions:**

| Category | Actions |
|---|---|
| `messages` | `list`, `create`, `send_attachment`, `get`, `delete` |
| `rooms` | `list`, `create`, `get`, `update`, `delete` |
| `teams` | `list`, `create`, `get`, `update` |
| `memberships` | `list`, `create`, `update`, `delete` |
| `meetings` | `list`, `create`, `get`, `update`, `delete` |
| `transcripts` | `list`, `download`, `list_snippets`, `get_snippet`, `update_snippet` |
| `webhooks` | `list`, `create`, `get`, `update`, `delete` |

#### Preset Flags

For convenience, two preset flags are available that automatically add a curated set of tools to the `--include` list:

- **`--minimal`** -- All operations for messages, rooms, teams, meetings, and transcripts (excludes memberships and webhooks). **23 tools.**
- **`--readonly-minimal`** -- Only read/list/get operations for messages, rooms, teams, meetings, and transcripts. No create, update, or delete. **12 tools.**

These flags **merge** with `--include` -- they don't override it. For example, `--minimal --include "webhooks:list"` registers the minimal set plus `webhooks:list`. If both `--minimal` and `--readonly-minimal` are set, `--minimal` takes priority.

**Examples:**

```bash
# Only register read-only transcript tools
./webex-go-mcp --include "transcripts:list,transcripts:download,transcripts:list_snippets,transcripts:get_snippet"

# Register all tools except destructive ones
./webex-go-mcp --exclude "messages:delete,rooms:delete,meetings:delete,memberships:delete,webhooks:delete"

# Use the minimal preset (messages, rooms, teams, meetings, transcripts)
./webex-go-mcp --minimal

# Use readonly-minimal (only read operations, no writes)
./webex-go-mcp --readonly-minimal

# Minimal preset plus an extra tool
./webex-go-mcp --minimal --include "webhooks:list"
```

## Usage

### Run directly

```bash
export WEBEX_ACCESS_TOKEN="your-token-here"
./webex-go-mcp
```

### Run directly from Git (no build required)

If you have Go installed, you can run the server directly from the repository without cloning or building. Go will fetch, compile, and execute in one step:

```bash
export WEBEX_ACCESS_TOKEN="your-token-here"
go run github.com/tejzpr/webex-go-mcp@latest
```

This is especially convenient when configuring MCP clients like Cursor or Claude Desktop -- no pre-built binary needed.

### Claude Desktop

Add to your Claude Desktop MCP configuration (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

**Using a pre-built binary:**

```json
{
  "mcpServers": {
    "webex": {
      "command": "/path/to/webex-go-mcp",
      "env": {
        "WEBEX_ACCESS_TOKEN": "your-token-here"
      }
    }
  }
}
```

**Using `go run` directly from Git (no build step):**

```json
{
  "mcpServers": {
    "webex": {
      "command": "go",
      "args": ["run", "github.com/tejzpr/webex-go-mcp@latest"],
      "env": {
        "WEBEX_ACCESS_TOKEN": "your-token-here"
      }
    }
  }
}
```

### Cursor

Add to your Cursor MCP configuration (`.cursor/mcp.json` in your project or `~/.cursor/mcp.json` globally):

**Using a pre-built binary:**

```json
{
  "mcpServers": {
    "webex": {
      "command": "/path/to/webex-go-mcp",
      "env": {
        "WEBEX_ACCESS_TOKEN": "your-token-here"
      }
    }
  }
}
```

**Using `go run` directly from Git (no build step):**

```json
{
  "mcpServers": {
    "webex": {
      "command": "go",
      "args": ["run", "github.com/tejzpr/webex-go-mcp@latest"],
      "env": {
        "WEBEX_ACCESS_TOKEN": "<WEBEX_ACCESS_TOKEN>"
      }
    }
  }
}
```

**With tool filtering (only expose specific tools):**

```json
{
  "mcpServers": {
    "webex": {
      "command": "go",
      "args": ["run", "github.com/tejzpr/webex-go-mcp@latest", "--include", "messages:list,messages:get,transcripts:list,transcripts:download"],
      "env": {
        "WEBEX_ACCESS_TOKEN": "<WEBEX_ACCESS_TOKEN>"
      }
    }
  }
}
```

**With readonly-minimal preset (safe, read-only access):**

```json
{
  "mcpServers": {
    "webex": {
      "command": "go",
      "args": ["run", "github.com/tejzpr/webex-go-mcp@latest", "--readonly-minimal"],
      "env": {
        "WEBEX_ACCESS_TOKEN": "<WEBEX_ACCESS_TOKEN>"
      }
    }
  }
}
```

> **Note:** The `go run` approach requires Go to be installed and available on your `PATH`. The first run will download and compile the module (cached for subsequent runs). To update to the latest version, Go will re-fetch when `@latest` resolves to a newer release.

## Tool Reference

### Messages

- **`webex_messages_list`** -- List messages in a room (requires `roomId`). Enriched with room context, sender names, and file metadata.
- **`webex_messages_create`** -- Send a text message. To DM someone, just pass `toPersonEmail` -- no room lookup needed. For group spaces, use `roomId`.
- **`webex_messages_send_attachment`** -- Send a message with a file attachment (public URL). Same destination options as create.
- **`webex_messages_get`** -- Get a message by ID. Enriched with sender profile, room info, and file content (text files inline).
- **`webex_messages_delete`** -- Delete a message by ID

### Rooms / Spaces

- **`webex_rooms_list`** -- List rooms (filter by `teamId`, `type`, `sortBy`)
- **`webex_rooms_create`** -- Create a room (`title` required, optional `teamId`)
- **`webex_rooms_get`** -- Get room details by ID
- **`webex_rooms_update`** -- Update room title
- **`webex_rooms_delete`** -- Delete a room

### Teams

- **`webex_teams_list`** -- List teams
- **`webex_teams_create`** -- Create a team (`name` required)
- **`webex_teams_get`** -- Get team details by ID
- **`webex_teams_update`** -- Update team name

### Memberships

- **`webex_memberships_list`** -- List memberships (filter by `roomId`, `personEmail`)
- **`webex_memberships_create`** -- Add person to room (`roomId` + `personEmail` or `personId`)
- **`webex_memberships_update`** -- Update membership (set `isModerator`)
- **`webex_memberships_delete`** -- Remove person from room

### Meetings

- **`webex_meetings_list`** -- List meetings (filter by `meetingType`, `state`, `from`, `to`). Note: `meetingType` is required when `state` is used.
- **`webex_meetings_create`** -- Schedule a meeting (`title`, `start`, `end` required)
- **`webex_meetings_get`** -- Get meeting details by ID
- **`webex_meetings_update`** -- Update a meeting
- **`webex_meetings_delete`** -- Cancel/delete a meeting

### Transcripts

- **`webex_transcripts_list`** -- List meeting transcripts (filter by `meetingId`, `hostEmail`, date range)
- **`webex_transcripts_download`** -- Download transcript content (requires `transcriptId` + `meetingId`, optional `format`: `txt` or `vtt`)
- **`webex_transcripts_list_snippets`** -- List spoken segments from a transcript
- **`webex_transcripts_get_snippet`** -- Get a specific transcript snippet
- **`webex_transcripts_update_snippet`** -- Update/correct a transcript snippet's text

### Webhooks

- **`webex_webhooks_list`** -- List webhooks
- **`webex_webhooks_create`** -- Create a webhook (`name`, `targetUrl`, `resource`, `event` required)
- **`webex_webhooks_get`** -- Get webhook details by ID
- **`webex_webhooks_update`** -- Update a webhook
- **`webex_webhooks_delete`** -- Delete a webhook

## Architecture

```
webex-go-mcp/
  main.go       -- Cobra CLI + Viper config, creates Webex SDK client
  server.go     -- MCP server setup, registers all tools, STDIO transport
  tools/
    filter.go         -- ToolRegistrar interface, tool include/exclude filtering
    messages.go       -- 4 message tools
    rooms.go          -- 5 room tools
    teams.go          -- 4 team tools
    memberships.go    -- 4 membership tools
    meetings.go       -- 5 meeting tools
    transcripts.go    -- 5 transcript tools
    webhooks.go       -- 5 webhook tools
```

## Dependencies

- [mcp-go](https://github.com/mark3labs/mcp-go) -- MCP server framework
- [webex-go-sdk](https://github.com/tejzpr/webex-go-sdk) -- Webex API client
- [cobra](https://github.com/spf13/cobra) -- CLI framework
- [viper](https://github.com/spf13/viper) -- Configuration management

## License

MPL-2.0

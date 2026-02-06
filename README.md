# Webex Go MCP Server

A Go-based STDIO [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server that exposes Cisco Webex APIs as tools. This allows LLMs (like Claude) to interact with Webex -- sending messages, managing rooms, scheduling meetings, downloading transcripts, and more.

## Features

**31 MCP tools** across 7 Webex API resource categories:

| Category | Tools | Operations |
|---|---|---|
| **Messages** | 4 | List, create, get, delete messages in rooms |
| **Rooms** | 5 | List, create, get, update, delete rooms/spaces |
| **Teams** | 4 | List, create, get, update teams |
| **Memberships** | 4 | List, create, update, delete room memberships |
| **Meetings** | 5 | List, create, get, update, delete scheduled meetings |
| **Transcripts** | 4 | List transcripts, download content, list/get snippets |
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

> **Note:** The `go run` approach requires Go to be installed and available on your `PATH`. The first run will download and compile the module (cached for subsequent runs). To update to the latest version, Go will re-fetch when `@latest` resolves to a newer release.

## Tool Reference

### Messages

- **`webex_messages_list`** -- List messages in a room (requires `roomId`)
- **`webex_messages_create`** -- Send a message (provide `roomId`, `toPersonId`, or `toPersonEmail` + `text` or `markdown`)
- **`webex_messages_get`** -- Get a message by ID
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

- **`webex_meetings_list`** -- List meetings (filter by `meetingType`, `state`, `from`, `to`)
- **`webex_meetings_create`** -- Schedule a meeting (`title`, `start`, `end` required)
- **`webex_meetings_get`** -- Get meeting details by ID
- **`webex_meetings_update`** -- Update a meeting
- **`webex_meetings_delete`** -- Cancel/delete a meeting

### Transcripts

- **`webex_transcripts_list`** -- List meeting transcripts (filter by `meetingId`, `hostEmail`, date range)
- **`webex_transcripts_download`** -- Download transcript content (`format`: `txt` or `vtt`)
- **`webex_transcripts_list_snippets`** -- List spoken segments from a transcript
- **`webex_transcripts_get_snippet`** -- Get a specific transcript snippet

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
    messages.go       -- 4 message tools
    rooms.go          -- 5 room tools
    teams.go          -- 4 team tools
    memberships.go    -- 4 membership tools
    meetings.go       -- 5 meeting tools
    transcripts.go    -- 4 transcript tools
    webhooks.go       -- 5 webhook tools
```

## Dependencies

- [mcp-go](https://github.com/mark3labs/mcp-go) -- MCP server framework
- [webex-go-sdk](https://github.com/tejzpr/webex-go-sdk) -- Webex API client
- [cobra](https://github.com/spf13/cobra) -- CLI framework
- [viper](https://github.com/spf13/viper) -- Configuration management

## License

MPL-2.0

# Fix: web_search 401 in headless/custom Ollama builds

## Problem

Building Ollama from source and running in headless mode, the web search
tool returns `search API error (status 401)` even when logged into an
ollama.com account with valid SSH keys at `~/.ollama/id_ed25519`.

The `auth.Sign()` function produces an SSH key signature, but ollama.com
rejects it for custom builds. The stock installed Ollama (which routes
web search through `/api/experimental/web_search` on the core server)
works fine — only the UI server's direct call to `ollama.com/api/web_search`
fails.

## Root Cause

The UI server's `web_search` tool in `app/tools/web_search.go` calls
`https://ollama.com/api/web_search` directly, authenticating with an SSH
key signature via `auth.Sign()`. This signature is rejected (401) for
builds from source, likely due to version or signing differences.

## Fix

Check for `OLLAMA_API_KEY` environment variable and prefer it over the
SSH signature when set.

### Code Change (`app/tools/web_search.go`)

```go
// Before (only SSH signature):
req.Header.Set("Content-Type", "application/json")
if signature != "" {
    req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", signature))
}

// After (prefer API key when set):
req.Header.Set("Content-Type", "application/json")
if apiKey := os.Getenv("OLLAMA_API_KEY"); apiKey != "" {
    req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
} else if signature != "" {
    req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", signature))
}
```

Same change applies to `app/tools/web_fetch.go`.

### Usage

```bash
OLLAMA_API_KEY="your-key-here" ./ollama-app --headless --port=3001
```

Get your API key from your ollama.com account settings.

### Logging Added

Debug logging was added to `performWebSearch()` to trace auth flow:

```
web_search: starting query="..." max_results=5
web_search: cloud check passed
web_search: auth has_signature=true has_api_key=true url="https://ollama.com/api/web_search?ts=..."
web_search: using OLLAMA_API_KEY for auth
web_search: success status=200
```

On failure, the full URL and response body are logged:

```
web_search: API error status=401 url="https://ollama.com/api/web_search?ts=..." body="{\"error\":\"unauthorized\"}"
```

## Files Modified

- `app/tools/web_search.go` — API key support + logging
- `app/tools/web_fetch.go` — needs same API key change (TODO)

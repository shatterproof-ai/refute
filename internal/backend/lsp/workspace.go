package lsp

// PrimeGoWorkspace opens every *.go file under workspaceRoot (skipping
// vendor/node_modules/.git and friends) via DidOpen so gopls can index their
// packages, then issues a zero-result WorkspaceSymbol call to drain the
// client's notification queue. Returns the number of files opened.
//
// The walk itself is the shared primeFiles routine, parameterized by the "go"
// language profile (file cap, skip dirs, sentinel round-trip).
func (c *Client) PrimeGoWorkspace(workspaceRoot string) (int, error) {
	p := profileFor("go").priming
	opened := primeFiles(c, workspaceRoot, p)
	if p.drainWithSentinel {
		// Round-trip to drain the notification queue.
		_, _ = c.WorkspaceSymbol("__refute_prime_sentinel__")
	}
	return opened, nil
}

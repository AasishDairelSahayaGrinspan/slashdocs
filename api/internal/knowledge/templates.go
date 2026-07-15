package knowledge

var BuiltinTemplates = []Template{
	{ID: "domain-context", Name: "Domain context", Description: "Capture concepts, invariants, ownership, and key flows.", Title: "Domain context", HTML: `<!doctype html><html><body><h1>Domain context</h1><h2>Purpose</h2><p>Describe the domain and the customer problem.</p><h2>Core concepts</h2><p>Define entities, invariants, and terminology.</p><h2>Key flows</h2><p>Document the important end-to-end flows.</p><h2>Ownership</h2><p>List owners and escalation paths.</p></body></html>`},
	{ID: "architecture", Name: "Architecture", Description: "Describe components, boundaries, data flows, and decisions.", Title: "Architecture", HTML: `<!doctype html><html><body><h1>Architecture</h1><h2>Overview</h2><p>Describe the system at a glance.</p><h2>Components</h2><p>List components and responsibilities.</p><h2>Data flow</h2><p>Explain inputs, processing, and outputs.</p><h2>Decisions</h2><p>Record important trade-offs.</p></body></html>`},
	{ID: "api", Name: "API guide", Description: "Document an API contract with examples and failure modes.", Title: "API guide", HTML: `<!doctype html><html><body><h1>API guide</h1><h2>Authentication</h2><p>Describe authentication.</p><h2>Endpoints</h2><p>Document requests and responses.</p><h2>Errors</h2><p>List failure modes and retries.</p><h2>Examples</h2><p>Add complete examples.</p></body></html>`},
	{ID: "runbook", Name: "Runbook", Description: "Operational symptoms, diagnosis, mitigation, and escalation.", Title: "Runbook", HTML: `<!doctype html><html><body><h1>Runbook</h1><h2>Symptoms</h2><p>What operators observe.</p><h2>Diagnosis</h2><p>Checks, dashboards, logs, and queries.</p><h2>Mitigation</h2><p>Safe recovery actions.</p><h2>Escalation</h2><p>Owners and escalation paths.</p></body></html>`},
	{ID: "onboarding", Name: "Onboarding", Description: "Help a new contributor become productive.", Title: "Onboarding", HTML: `<!doctype html><html><body><h1>Onboarding</h1><h2>Prerequisites</h2><p>Access and tools.</p><h2>Local setup</h2><p>Steps to run the project.</p><h2>First change</h2><p>How to test and submit work.</p><h2>Where to get help</h2><p>Owners and support channels.</p></body></html>`},
}

func TemplateByID(id string) (Template, bool) {
	for _, item := range BuiltinTemplates {
		if item.ID == id {
			return item, true
		}
	}
	return Template{}, false
}

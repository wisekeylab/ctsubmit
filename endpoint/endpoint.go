package endpoint

type Endpoint int

const (
	// POST (API) and GET (webpage).
	ENDPOINTSTRING_ADDCHAIN    = "add-chain"
	ENDPOINTSTRING_ADDPRECHAIN = "add-pre-chain"

	// GET.
	ENDPOINTSTRING_FRONTPAGE      = ""
	ENDPOINTSTRING_CSS            = "ctsubmit.css"
	ENDPOINTSTRING_DASHBOARD      = "dashboard"
	ENDPOINTSTRING_FAVICON        = "favicon.ico"
	ENDPOINTSTRING_MASCOT         = "mascot.png"
	ENDPOINTSTRING_USABLETLSLOGS  = "usable_tls_logs.json"
	ENDPOINTSTRING_ACTIVETLSLOGS  = "active_tls_logs.json"
	ENDPOINTSTRING_TESTTLSLOGS    = "test_tls_logs.json"
	ENDPOINTSTRING_USABLEBIMILOGS = "usable_bimi_logs.json"

	// GET (Monitoring).
	ENDPOINTSTRING_LIVEZ   = "livez"
	ENDPOINTSTRING_READYZ  = "readyz"
	ENDPOINTSTRING_METRICS = "metrics"
	ENDPOINTSTRING_BUILD   = "debug/build"
	ENDPOINTSTRING_CONFIG  = "debug/config"
)

const (
	ENDPOINT_ADDCHAIN Endpoint = iota
	ENDPOINT_ADDPRECHAIN
)

var postEndpoint = map[string]Endpoint{
	ENDPOINTSTRING_ADDCHAIN:    ENDPOINT_ADDCHAIN,
	ENDPOINTSTRING_ADDPRECHAIN: ENDPOINT_ADDPRECHAIN,
}

var APIEndpoint = map[Endpoint]string{
	ENDPOINT_ADDCHAIN:    ENDPOINTSTRING_ADDCHAIN,
	ENDPOINT_ADDPRECHAIN: ENDPOINTSTRING_ADDPRECHAIN,
}

func CheckPOSTEndpoint(endpointString string) (apiEndpoint Endpoint, ok bool) {
	apiEndpoint, ok = postEndpoint[endpointString]
	return
}

package request

import (
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/crtsh/ctsubmit/config"
	"github.com/crtsh/ctsubmit/endpoint"
	"github.com/crtsh/ctsubmit/health"
	"github.com/crtsh/ctsubmit/logger"
	"github.com/crtsh/ctsubmit/loglists"
	"github.com/crtsh/ctsubmit/submitter"
	"github.com/crtsh/ctsubmit/utils"

	"filippo.io/sunlight"

	json "github.com/goccy/go-json"
	"github.com/google/certificate-transparency-go/tls"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/valyala/fasthttp"

	"go.uber.org/zap"

	"schneider.vip/problem"
)

var endpointRequestCounter = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: config.ApplicationNamespace,
	Subsystem: "endpoint",
	Name:      "requests_total",
	Help:      "Total requests per submission endpoint, by result.",
}, []string{"endpoint", "result"})

func getResponseFormat(fhctx *fasthttp.RequestCtx) config.ResponseFormat {
	if f := paramS(fhctx, "format"); f != "" {
		return config.ParseResponseFormat(f)
	} else {
		switch utils.B2S(fhctx.Request.Header.Peek("Accept")) {
		case "text/html":
			return config.RESPONSEFORMAT_HTML
		case "application/json":
			return config.RESPONSEFORMAT_JSON
		}
	}

	return config.DefaultResponseFormat
}

func POST(fhctx *fasthttp.RequestCtx, path string) int {
	status := fasthttp.StatusBadRequest

	ctxWithDeadline, cancel := context.WithDeadline(context.Background(), fhctx.Time().Add(time.Duration(config.Config.Server.RequestTimeout)))
	defer cancel()

	doneChan := make(chan int, 1)
	go func() {
		submissionRequest := submitter.NewSubmissionRequest()
		var submissionResponse *submitter.SubmissionResponse
		var err error
		var responseFormat config.ResponseFormat
		if apiEndpoint, ok := endpoint.CheckPOSTEndpoint(path); !ok {
			status = fasthttp.StatusNotFound
			logger.SetDetails(fhctx, zap.InfoLevel, "Invalid endpoint", nil, nil)
		} else if responseFormat = getResponseFormat(fhctx); responseFormat == -1 {
			err = fmt.Errorf("Unrecognised response format")
		} else if requestBody := fhctx.Request.Body(); len(requestBody) == 0 {
			err = fmt.Errorf("Empty request body")
		} else if err = json.Unmarshal(requestBody, submissionRequest); err == nil {
			submissionResponse, err = submitter.Handler(fhctx, ctxWithDeadline, apiEndpoint, submissionRequest)
			if err == nil {
				status = fasthttp.StatusOK
			}
		}

		if apiEndpoint, ok := endpoint.CheckPOSTEndpoint(path); ok {
			result := "success"
			if err != nil {
				result = "error"
			}
			endpointRequestCounter.WithLabelValues(endpoint.APIEndpoint[apiEndpoint], result).Inc()
		}

		logger.SetDetails(fhctx, zap.InfoLevel, "Submission Request", err, nil)

		// Add Cross-Origin Resource Sharing (CORS) response header.
		fhctx.Response.Header.Set("Access-Control-Allow-Origin", "*")

		// Send response.
		switch responseFormat {
		case config.RESPONSEFORMAT_HTML:
			status = sendHTMLResponse(fhctx, submissionResponse, err)
		case config.RESPONSEFORMAT_JSON:
			if err == nil {
				status = sendJSONResponse(fhctx, submissionResponse)
			} else {
				status = sendJSONProblem(fhctx, status, err)
			}
		}
		fhctx.SetStatusCode(status)
		doneChan <- 0
	}()

	return health.CompleteRequest(ctxWithDeadline, doneChan)
}

func paramS(fhctx *fasthttp.RequestCtx, name string) string {
	return utils.B2S(paramB(fhctx, name))
}

func paramB(fhctx *fasthttp.RequestCtx, name string) []byte {
	if arg := fhctx.PostArgs().Peek(name); len(arg) > 0 {
		return arg
	} else if arg = fhctx.QueryArgs().Peek(name); len(arg) > 0 {
		return arg
	} else if form, err := fhctx.MultipartForm(); err == nil {
		if s := form.Value[name]; len(s) > 0 {
			return utils.S2B(s[0])
		}
	}

	return nil
}

func sendHTMLResponse(fhctx *fasthttp.RequestCtx, submissionResponse *submitter.SubmissionResponse, err error) int {
	fhctx.SetContentType("text/html; charset=UTF-8")

	var h strings.Builder
	h.WriteString(`<TABLE style="border:1px solid #CCCCCC;font:8pt 'Roboto Mono',monospace">`)

	if err != nil {
		h.WriteString(`<TR><TH>Error</TH></TR>`)
		h.WriteString(`<TR><TD style="color:red">`)
		h.WriteString(html.EscapeString(err.Error()))
		h.WriteString(`</TD></TR>`)
		h.WriteString(`</TABLE>`)
		fhctx.SetBodyString(h.String())
		return fasthttp.StatusBadRequest
	}

	// SCT responses.
	if len(submissionResponse.LogResponse) > 0 {
		h.WriteString(`<TR><TH colspan="2" style="border-bottom:1px solid #CCCCCC;border-top:2px solid #CCCCCC">SCTs (`)
		h.WriteString(fmt.Sprintf("%d", len(submissionResponse.LogResponse)))
		h.WriteString(`)</TH></TR>`)
		for i, sct := range submissionResponse.LogResponse {
			if i > 0 {
				h.WriteString(`<TR><TD colspan="2" style="border-top:1px solid #EEEEEE"></TD></TR>`)
			}
			h.WriteString(`<TR><TD>Version</TD><TD>`)
			h.WriteString(fmt.Sprintf("%d &nbsp;<I>(%s)</I>", sct.SCTVersion, sct.SCTVersion.String()))
			h.WriteString(`</TD></TR>`)
			h.WriteString(`<TR><TD>Log ID</TD><TD>`)
			h.WriteString(html.EscapeString(base64.StdEncoding.EncodeToString(sct.ID)))
			operatorName, logName := loglists.GetLogName(sct.ID)
			if logName != "" {
				h.WriteString(` &nbsp;<I>(`)
				if operatorName != "" {
					h.WriteString(html.EscapeString(operatorName))
					h.WriteString(` `)
				}
				h.WriteString(html.EscapeString(logName))
				h.WriteString(`)</I>`)
			}
			h.WriteString(`</TD></TR>`)
			h.WriteString(`<TR><TD>Timestamp</TD><TD>`)
			h.WriteString(fmt.Sprintf("%d &nbsp;<I>(%s)</I>", sct.Timestamp, time.UnixMilli(int64(sct.Timestamp)).UTC().Format(time.RFC3339)))
			h.WriteString(`</TD></TR>`)
			if sct.Extensions != "" {
				h.WriteString(`<TR><TD>Extensions</TD><TD style="word-break:break-all">`)
				h.WriteString(html.EscapeString(sct.Extensions))
				extBytes, err := base64.StdEncoding.DecodeString(sct.Extensions)
				if err == nil {
					if ext, err := sunlight.ParseExtensions(extBytes); err == nil {
						h.WriteString(fmt.Sprintf(" &nbsp;<I>(Leaf Index: %d)</I>", ext.LeafIndex))
					}
				}
				h.WriteString(`</TD></TR>`)
			}
			h.WriteString(`<TR><TD>Signature</TD><TD style="word-break:break-all">`)
			sigB64 := base64.StdEncoding.EncodeToString(sct.Signature)
			mid := len(sigB64) / 2
			h.WriteString(html.EscapeString(sigB64[:mid]))
			h.WriteString(`<BR>`)
			h.WriteString(html.EscapeString(sigB64[mid:]))
			var ds tls.DigitallySigned
			if rest, err := tls.Unmarshal(sct.Signature, &ds); err == nil && len(rest) == 0 {
				h.WriteString(fmt.Sprintf(` &nbsp;<I>(%s / %s)</I>`, ds.Algorithm.Hash, ds.Algorithm.Signature))
			}
			h.WriteString(`</TD></TR>`)
		}
	}

	// Final TBS Certificate.
	if submissionResponse.FinalTBSCertB64 != "" {
		h.WriteString(`<TR><TH colspan="2" style="border-bottom:1px solid #CCCCCC;border-top:2px solid #CCCCCC">Final TBS Certificate</TH></TR>`)
		h.WriteString(`<TR><TD colspan="2" style="word-break:break-all">`)
		s := submissionResponse.FinalTBSCertB64
		for len(s) > 64 {
			h.WriteString(html.EscapeString(s[:64]))
			h.WriteString(`<BR>`)
			s = s[64:]
		}
		h.WriteString(html.EscapeString(s))
		h.WriteString(`</TD></TR>`)
	}

	// CTLint findings.
	if len(submissionResponse.CTLint) > 0 {
		h.WriteString(`<TR><TH colspan="2" style="border-bottom:1px solid #CCCCCC;border-top:2px solid #CCCCCC">CT Lint</TH></TR>`)
		for _, finding := range submissionResponse.CTLint {
			color := ""
			switch finding.Severity {
			case "ERROR", "FATAL":
				color = `style="color:red"`
			case "WARNING":
				color = `style="color:orange"`
			}
			h.WriteString(`<TR><TD `)
			h.WriteString(color)
			h.WriteString(`>`)
			h.WriteString(html.EscapeString(finding.Severity))
			h.WriteString(`</TD><TD>`)
			h.WriteString(html.EscapeString(finding.Finding))
			h.WriteString(`</TD></TR>`)
		}
	}

	// Strategy details.
	if len(submissionResponse.Strategy) > 0 {
		h.WriteString(`<TR><TH colspan="2" style="border-bottom:1px solid #CCCCCC;border-top:2px solid #CCCCCC">Strategy</TH></TR>`)
		h.WriteString(`<TR style="font-weight:bold"><TD>Log</TD><TD>Bucket &rarr; Outcome (Latency)</TD></TR>`)
		for _, sm := range submissionResponse.Strategy {
			h.WriteString(`<TR><TD>`)
			h.WriteString(html.EscapeString(sm.Operator))
			h.WriteString(` `)
			if sm.LogName != "" {
				h.WriteString(html.EscapeString(sm.LogName))
			} else {
				h.WriteString(html.EscapeString(sm.SubmissionURL))
			}
			h.WriteString(`</TD><TD>`)
			// Bucket.
			bucketJSON, _ := sm.Bucket.MarshalJSON()
			h.WriteString(html.EscapeString(string(bucketJSON)))
			// Outcome.
			if sm.Outcome != "" {
				h.WriteString(` &rarr; `)
				h.WriteString(html.EscapeString(sm.Outcome))
			}
			// Timing.
			if sm.TimeTaken > 0 {
				h.WriteString(fmt.Sprintf(" (%.3fs)", sm.TimeTaken.Seconds()))
			}
			h.WriteString(`</TD></TR>`)
		}
	}

	h.WriteString(`</TABLE>`)
	fhctx.SetBodyString(h.String())
	return fasthttp.StatusOK
}

func sendJSONResponse(fhctx *fasthttp.RequestCtx, submissionResponse *submitter.SubmissionResponse) int {
	// Encode and send the results as JSON.
	fhctx.SetContentType("application/json; charset=UTF-8")

	j := json.NewEncoder(fhctx)
	j.SetEscapeHTML(false)
	if config.Config.Response.JsonPrettyPrint {
		j.SetIndent("", "  ")
	}
	if err := j.Encode(submissionResponse); err != nil {
		logger.SetDetails(fhctx, zap.ErrorLevel, "Failed to encode JSON", nil, nil)
	}

	return fasthttp.StatusOK
}

func sendJSONProblem(fhctx *fasthttp.RequestCtx, status int, err error) int {
	// Encode and send the error as a JSON Problem response.
	fhctx.SetContentType(problem.ContentTypeJSON)
	fhctx.SetBody(problem.Of(status).Append(problem.Detail(err.Error())).JSON())

	return status
}

package request

import (
	"github.com/crtsh/ctsubmit/logger"

	json "github.com/goccy/go-json"
	"github.com/google/certificate-transparency-go/loglist3"
	"github.com/valyala/fasthttp"

	"go.uber.org/zap"
)

func LogList(fhctx *fasthttp.RequestCtx, logList *loglist3.LogList, logListDescription string) {
	logger.SetDetails(fhctx, zap.InfoLevel, logListDescription, nil, nil)

	body, err := json.MarshalIndent(*logList, "", "  ")
	if err != nil {
		logger.SetDetails(fhctx, zap.ErrorLevel, "Failed to marshal log list", err, nil)
		fhctx.SetStatusCode(fasthttp.StatusInternalServerError)
		return
	}
	fhctx.SetBody(body)

	fhctx.SetContentType("application/json")
	fhctx.SetStatusCode(fasthttp.StatusOK)
}

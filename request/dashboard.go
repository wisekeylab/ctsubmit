package request

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/crtsh/ctsubmit/config"
	"github.com/crtsh/ctsubmit/endpoint"
	"github.com/crtsh/ctsubmit/logger"
	"github.com/crtsh/ctsubmit/loglists"
	"github.com/crtsh/ctsubmit/monitor"
	"github.com/crtsh/ctsubmit/utils"

	"github.com/google/certificate-transparency-go/loglist3"
	"github.com/valyala/fasthttp"

	"go.uber.org/zap"
)

func Dashboard(fhctx *fasthttp.RequestCtx) {
	var logList *loglist3.LogList
	logListName := ""
	switch paramS(fhctx, "loglist") {
	case "usabletls", "": // default
		logList = loglists.UsableTLSLogs
		logListName = "Usable TLS Log"
	case "activetls":
		logList = loglists.ActiveTLSLogs
		logListName = "Active TLS Log"
	case "testtls":
		logList = loglists.TestTLSLogs
		logListName = "Test TLS Log"
	case "usablebimi":
		logList = loglists.UsableBIMILogs
		logListName = "Usable BIMI Log"
	default:
		fhctx.NotFound()
		logger.SetDetails(fhctx, zap.InfoLevel, "Invalid loglist query parameter", nil, nil)
		return
	}

	var response strings.Builder
	response.WriteString(`<!DOCTYPE HTML>
<HTML>
<HEAD>
  <META http-equiv="Content-Type" content="text/html; charset=UTF-8">
  <TITLE>`)
	response.WriteString(config.ApplicationName)
	response.WriteString(` | CT Submission Proxy</TITLE>
  <LINK href="https://fonts.googleapis.com/css2?family=Urbanist:wght@500;700&family=DM+Sans:wght@400;500&family=Roboto+Mono&family=Roboto:wght@400;500" rel="stylesheet">
  <LINK href="/`)
	response.WriteString(endpoint.ENDPOINTSTRING_CSS)
	response.WriteString(`" rel="stylesheet">
  <STYLE type="text/css">
    table tr:nth-child(2n+4) {
      background: #E7E7E7
    }
  </STYLE>
</HEAD>
<BODY>
  <TABLE>
    <TR>
      <TD style="text-align:center;vertical-align:middle"><A href="https://`)
	response.WriteString(utils.GetPackagePath())
	response.WriteString(`/releases" target="_blank"><DIV class="title">`)
	response.WriteString(config.ApplicationName)
	response.WriteString(` `)
	response.WriteString(config.VcsRevision)
	response.WriteString(`</DIV></A></TD>
      <TD style="padding-left:50px"><A href="https://`)
	response.WriteString(utils.GetPackagePath())
	response.WriteString(`" target="_blank"><IMG src="/`)
	response.WriteString(endpoint.ENDPOINTSTRING_MASCOT)
	response.WriteString(`" width="100" height="100"></A></TD>
    </TR>
    <TR>
      <TD>
        <TABLE style="border: 1px solid #CCCCCC; font: 10pt 'Roboto Mono', monospace">
          <TR style="font-size:12pt">
            <TH rowspan="2">Operator</TH>
            <TH colspan="3" style="border-left:2px solid #CCCCCC">`)
	response.WriteString(logListName)
	response.WriteString(`</TH>
            <TH colspan="2" style="border-left:2px solid #CCCCCC">STH</TH>
			<TH colspan="2" style="border-left:2px solid #CCCCCC">Uptime</TH>
			<TH colspan="3" style="border-left:2px solid #CCCCCC">Submissions (Last 30s)</TH>
			<TH colspan="2" style="border-left:2px solid #CCCCCC">Dispreferring</TH>
          </TR>
          <TR style="border-bottom:1px solid #CCCCCC">
            <TH style="border-left:2px solid #CCCCCC">Name [URL]</TH>
			<TH>Type/API</TH>
            <TH>Tree Size</TH>
            <TH style="border-left:2px solid #CCCCCC">MMD</TH>
            <TH>Age</TH>
            <TH style="border-left:2px solid #CCCCCC">24h %</TH>
            <TH>90d %</TH>
			<TH style="border-left:2px solid #CCCCCC">OK</TH>
			<TH>Fail</TH>
			<TH>Avg.Latency</TH>
			<TH style="border-left:2px solid #CCCCCC">T-minus</TH>
			<TH>Reason</TH>
          </TR>`)

	for _, operator := range logList.Operators {
		for _, log := range operator.Logs {
			response.WriteString(logDetails(operator.Name, log.Description, log.URL, log.URL, "RFC6962", log.MMD))
		}

		for _, tiledLog := range operator.TiledLogs {
			response.WriteString(logDetails(operator.Name, tiledLog.Description, tiledLog.MonitoringURL, tiledLog.SubmissionURL, "Static CT", tiledLog.MMD))
		}
	}

	response.WriteString(`
        </TABLE>
      </TD>
      <TD style="vertical-align:middle;padding-left:50px">`)
	response.WriteString(dashboards())
	response.WriteString(`
      </TD>
    </TR>
    <TR>
      <TD style="padding-top:25px;text-align:middle;vertical-align:bottom">`)
	response.WriteString(copyright())
	response.WriteString(`</TD><TD style="padding-left:50px">`)
	response.WriteString(builtAt())
	response.WriteString(`</TD>
    </TR>
  </TABLE>
</BODY>
</HTML>
`)

	logger.SetDetails(fhctx, zap.InfoLevel, "Dashboard", nil, nil)
	fhctx.SetBodyString(response.String())
	fhctx.SetContentType("text/html")
	fhctx.SetStatusCode(fasthttp.StatusOK)
}

func logDetails(operatorName, logDescription, monitoringURL, submissionURL, logType string, mmd int32) string {
	sthAgeString := "?"
	treeSizeString := "?"
	logURL, _ := url.JoinPath(monitoringURL, "/")
	sd, ok := monitor.GetSTHData(logURL)
	if ok {
		if sd.Timestamp != nil {
			sthAgeString = ageString(int32(time.Since(*sd.Timestamp)/time.Second), mmd)
		}
		if sd.LastFetched != nil {
			treeSizeString = fmt.Sprintf("%d", sd.TreeSize)
		}
	}

	logURL, _ = url.JoinPath(submissionURL, "/")
	var percentage24h, percentage90d string
	up24h, ok24 := monitor.GetEndpointUptime24h(logURL, "LOWEST")
	if ok24 {
		percentage24h = percentageString(up24h, config.Config.Strategy.UptimeThreshold.SubmitEndpoint24h, 0)
	}
	up90d, ok90 := monitor.GetEndpointUptime90d(logURL, "LOWEST")
	if ok90 {
		percentage90d = percentageString(up90d, config.Config.Strategy.UptimeThreshold.LowestEndpoint90d, 99)
	}

	backoff, _ := monitor.GetBadResponseBackoff(logURL)
	reason := "Bad Response"
	statusCode := 0
	if backoff <= 0 {
		backoff, _ = monitor.GetTimeoutBackoff(logURL)
		reason = "Timeout"
	}
	if backoff <= 0 {
		backoff, _, statusCode = monitor.Get5xxBackoff(logURL)
		reason = fmt.Sprintf("HTTP %d", statusCode)
	}
	if backoff <= 0 {
		backoff, _, statusCode = monitor.Get4xxBackoff(logURL)
		reason = fmt.Sprintf("HTTP %d", statusCode)
	}
	if backoff <= 0 {
		backoff, _ = monitor.GetSlowResponseBackoff(logURL)
		reason = "Slow Response"
	}
	backoffAgeString := ageString(int32(backoff/time.Second), int32(backoff/time.Second))
	if backoff <= 0 {
		backoffAgeString = ""
		reason = ""
	}

	successes, failures := monitor.GetRecentOutcomeCounts(logURL)
	successString := ""
	failureString := ""
	if successes > 0 || failures > 0 {
		successString = fmt.Sprintf("%d", successes)
		if failures > 0 {
			failureString = fmt.Sprintf(`<span style="color:red">%d</span>`, failures)
		} else {
			failureString = "0"
		}
	}

	responseTimeString := ""
	if rt, ok := monitor.GetAvgResponseTime(logURL); ok {
		responseTimeString = fmt.Sprintf("%.3fs", rt.Seconds())
	}

	return `
          <TR>
            <TD>` + operatorName + `</TD>
            <TD style="border-left:2px solid #CCCCCC"><A title="` + monitoringURL + `">` + logDescription + `</TD>
            <TD>` + logType + `</TD>
            <TD>` + treeSizeString + `</TD>
            <TD style="border-left:2px solid #CCCCCC">` + ageString(mmd, mmd) + `</TD>
            <TD>` + sthAgeString + `</TD>
            <TD style="border-left:2px solid #CCCCCC">` + percentage24h + `</TD>
            <TD>` + percentage90d + `</TD>
            <TD style="border-left:2px solid #CCCCCC">` + successString + `</TD>
            <TD>` + failureString + `</TD>
            <TD>` + responseTimeString + `</TD>
            <TD style="border-left:2px solid #CCCCCC">` + backoffAgeString + `</TD>
            <TD>` + reason + `</TD>
          </TR>`
}

func ageString(nSeconds, errorThreshold int32) string {
	if nSeconds < 0 {
		return ""
	}

	var s string
	if nSeconds >= 3600 {
		s = fmt.Sprintf("%dh", nSeconds/3600)
	}
	if (nSeconds%3600)/60 > 0 {
		s = fmt.Sprintf("%s%dm", s, (nSeconds%3600)/60)
	}
	if nSeconds%60 > 0 || s == "" {
		s = fmt.Sprintf("%s%ds", s, nSeconds%60)
	}
	if nSeconds > errorThreshold {
		s = fmt.Sprintf("<span style=\"color:red\">%s</span>", s)
	}
	return s
}

func percentageString(value, warningThreshold, errorThreshold float64) string {
	s := fmt.Sprintf("%.4f", value)
	if value < errorThreshold {
		s = fmt.Sprintf("<span style=\"color:red\">%s</span>", s)
	} else if value < warningThreshold {
		s = fmt.Sprintf("<span style=\"color:orange\">%s</span>", s)
	}
	return s
}

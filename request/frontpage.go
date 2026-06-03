package request

import (
	"strings"

	"github.com/crtsh/ctsubmit/config"
	"github.com/crtsh/ctsubmit/endpoint"
	"github.com/crtsh/ctsubmit/logger"
	"github.com/crtsh/ctsubmit/utils"

	"github.com/valyala/fasthttp"

	"go.uber.org/zap"
)

func FrontPage(fhctx *fasthttp.RequestCtx) {
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
        <BR><A href="https://`)
	response.WriteString(utils.GetPackagePath())
	response.WriteString(`/blob/main/doc/REST_API.md" target="_blank">REST API Documentation [GitHub]</A>
        <BR><BR><BR><B>Example webpages that use the submission REST APIs:</B>
        <UL>
          <LI><A href="/`)
	response.WriteString(endpoint.ENDPOINTSTRING_ADDCHAIN)
	response.WriteString(`">`)
	response.WriteString(endpoint.ENDPOINTSTRING_ADDCHAIN)
	response.WriteString(`</A> - Submit a Certificate</LI>
          <LI><A href="/`)
	response.WriteString(endpoint.ENDPOINTSTRING_ADDPRECHAIN)
	response.WriteString(`">`)
	response.WriteString(endpoint.ENDPOINTSTRING_ADDPRECHAIN)
	response.WriteString(`</A> - Submit a Precertificate</LI>
        </UL>
        <BR><B>Log lists:</B>
        <UL>
          <LI><A href="/`)
	response.WriteString(endpoint.ENDPOINTSTRING_USABLETLSLOGS)
	response.WriteString(`">`)
	response.WriteString(endpoint.ENDPOINTSTRING_USABLETLSLOGS)
	response.WriteString(`</A> - Usable TLS Logs</LI>
          <LI><A href="/`)
	response.WriteString(endpoint.ENDPOINTSTRING_ACTIVETLSLOGS)
	response.WriteString(`">`)
	response.WriteString(endpoint.ENDPOINTSTRING_ACTIVETLSLOGS)
	response.WriteString(`</A> - Active TLS Logs</LI>
          <LI><A href="/`)
	response.WriteString(endpoint.ENDPOINTSTRING_TESTTLSLOGS)
	response.WriteString(`">`)
	response.WriteString(endpoint.ENDPOINTSTRING_TESTTLSLOGS)
	response.WriteString(`</A> - Test TLS Logs</LI>
          <LI><A href="/`)
	response.WriteString(endpoint.ENDPOINTSTRING_USABLEBIMILOGS)
	response.WriteString(`">`)
	response.WriteString(endpoint.ENDPOINTSTRING_USABLEBIMILOGS)
	response.WriteString(`</A> - Usable BIMI Logs</LI>
        </UL>
      </TD>
      <TD style="vertical-align:bottom;padding-left:50px">`)
	response.WriteString(dashboards())
	response.WriteString(`<BR><BR>`)
	response.WriteString(builtAt())
	response.WriteString(`<BR></TD>
    </TR>
  </TABLE>
  <BR><BR><BR>`)
	response.WriteString(copyright())
	response.WriteString(`
</BODY>
</HTML>
`)

	logger.SetDetails(fhctx, zap.InfoLevel, "Front page", nil, nil)
	fhctx.SetBodyString(response.String())
	fhctx.SetContentType("text/html")
	fhctx.SetStatusCode(fasthttp.StatusOK)
}

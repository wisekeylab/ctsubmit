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

func APIWebpage(fhctx *fasthttp.RequestCtx, endpointPath string) {
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
	response.WriteString(utils.VersionString(config.CtsubmitVersion))
	response.WriteString(`</DIV></A></TD>
        <TD><A href="https://`)
	response.WriteString(utils.GetPackagePath())
	response.WriteString(`" target="_blank"><IMG src="/`)
	response.WriteString(endpoint.ENDPOINTSTRING_MASCOT)
	response.WriteString(`" width="100" height="100"></A></TD>
      </TR>
      <TR>
        <TD><B>`)
	inputType := ""
	firstItem := ""
	switch endpointPath {
	case endpoint.ENDPOINTSTRING_ADDCHAIN:
		inputType = "Certificate"
		firstItem = "leaf"
	case endpoint.ENDPOINTSTRING_ADDPRECHAIN:
		inputType = "Precertificate"
		firstItem = "precertificate"
	}
	response.WriteString(inputType)
	response.WriteString(` Chain (PEM):</B>
          <BR><TEXTAREA id="pemInput" cols="74" rows="36" autofocus autoCorrect="off" autoCapitalize="off" spellCheck="false" placeholder="Paste one or more PEM certificates forming a chain (`)
	response.WriteString(firstItem)
	response.WriteString(` first)"></TEXTAREA>
        </TD>
        <TD><B>Response Format:</B>
          <BR><SELECT id="format" size="2" style="overflow:hidden">
            <OPTION value="html" selected>html</OPTION>
            <OPTION value="json">json</OPTION>
          </SELECT>
          <BR><BR><BR><BR><BR><BR><BR><BR><BR><BR>
          <INPUT class="button" type="button" value="Submit" onclick="submitChain()">
        </TD>
      </TR>
      <TR><TD><HR></TD></TR>
      <TR>
        <TD><B>Options:</B>
          <BR><TABLE style="font-size:10pt;margin-left:0">
            <TR>
              <TD><INPUT type="checkbox" id="discoverChain" checked> Discover certificate chain?</TD>
              <TD>&nbsp;</TD>
            </TR>
            <TR>
              <TD><INPUT type="checkbox" id="policyCompliant" checked onchange="if(this.checked){document.getElementById('testLogs').checked=false;document.getElementById('requireAtLeastOneRFC6962SCT').checked=true;document.getElementById('preferAtLeastOneStaticSCT').checked=true}"> Require policy-compliant SCT list?</TD>
              <TD style="padding-left:20px"><INPUT type="checkbox" id="requireAtLeastOneRFC6962SCT" checked onchange="if(!this.checked)document.getElementById('policyCompliant').checked=false"> Require ≥1 RFC6962 SCT?</TD>
            </TR>
            <TR>
              <TD><INPUT type="checkbox" id="testLogs" onchange="if(this.checked)document.getElementById('policyCompliant').checked=false"> Use <A href="https://github.com/google/certificate-transparency-community-site/blob/master/docs/google/known-logs.md#test-logs">test logs</A>?</TD>
              <TD style="padding-left:20px"><INPUT type="checkbox" id="preferAtLeastOneStaticSCT" checked> Prefer ≥1 Static SCT?</TD>
            </TR>
            <TR>
              <TD><INPUT type="checkbox" id="mimics"> Include SCTs from <A href="https://googlechrome.github.io/CertificateTransparency/3p_libraries.html">log mimics</A>?</TD>
              <TD style="padding-left:20px"><INPUT type="checkbox" id="verbose" checked> Show strategy?</TD>
            </TR>
          </TABLE>
        </TD>
        <TD style="vertical-align:bottom">`)
	response.WriteString(builtAt())
	response.WriteString(`
        </TD>
      </TR>
      <TR style="background:transparent">
        <TD colspan="2"><B id="responseTitle" style="display:none"><BR><HR><BR>Response:</B><PRE id="response" style="white-space:pre-wrap;word-break:break-all;font-size:8pt;max-width:128ch"></PRE></TD>
      </TR>
    </TABLE>
  <SCRIPT>
  function submitChain() {
    const pem = document.getElementById("pemInput").value.trim();
    if (!pem) { alert("Please paste one or more PEM certificates."); return; }

    // Extract base64-encoded DER from each PEM block.
    const pemRegex = /-----BEGIN CERTIFICATE-----\s*([\s\S]*?)\s*-----END CERTIFICATE-----/g;
    const chain = [];
    let m;
    while ((m = pemRegex.exec(pem)) !== null) {
      chain.push(m[1].replace(/\s+/g, ""));
    }
    if (chain.length === 0) { alert("No PEM certificate blocks found."); return; }

    const format = document.getElementById("format").value;
    const url = "/`)
	response.WriteString(endpointPath)
	response.WriteString(`?format=" + encodeURIComponent(format);

    const pre = document.getElementById("response");
    pre.textContent = "Submitting...";

    const xhr = new XMLHttpRequest();
    xhr.open("POST", url);
    xhr.setRequestHeader("Content-Type", "application/json");
    xhr.onload = function() {
      document.getElementById("responseTitle").style.display = "";
	  let responseText;
      if (format === "json") {
		responseText = JSON.stringify(JSON.parse(xhr.responseText), null, 2);
        pre.textContent = responseText;
      } else {
	    responseText = xhr.responseText;
        try { pre.innerHTML = responseText; } catch(e) { pre.textContent = responseText; }
      }
    };
    xhr.onerror = function() {
      pre.textContent = "Request failed: " + xhr.statusText;
    };
    xhr.send(JSON.stringify({
      chain: chain,
      discoverChain: document.getElementById("discoverChain").checked,
      policyCompliant: document.getElementById("policyCompliant").checked,
      mimics: document.getElementById("mimics").checked,
      testLogs: document.getElementById("testLogs").checked,
      requireAtLeastOneRFC6962SCT: document.getElementById("requireAtLeastOneRFC6962SCT").checked,
      preferAtLeastOneStaticSCT: document.getElementById("preferAtLeastOneStaticSCT").checked,
      verbose: document.getElementById("verbose").checked
    }));
  }
  </SCRIPT><BR>`)
	response.WriteString(copyright())
	response.WriteString(`
</BODY>
</HTML>
`)

	logger.SetDetails(fhctx, zap.InfoLevel, endpointPath+" webpage", nil, nil)
	fhctx.SetBodyString(response.String())
	fhctx.SetContentType("text/html")
	fhctx.SetStatusCode(fasthttp.StatusOK)
}

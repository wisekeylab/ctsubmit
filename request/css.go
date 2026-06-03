package request

import (
	"strings"

	"github.com/crtsh/ctsubmit/logger"

	"github.com/valyala/fasthttp"

	"go.uber.org/zap"
)

func CSS(fhctx *fasthttp.RequestCtx) {
	var response strings.Builder
	response.WriteString(`
div {
  font: 8pt 'DM Sans', sans-serif;
  font-style: italic
}
table {
  border-collapse: collapse;
  color: #222222;
  font: 12pt 'DM Sans', sans-serif;
  margin-left: auto;
  margin-right: auto
}
td, th {
  padding: 0px 10px;
  text-align: left;
  vertical-align: top
}
select {
  font: 10pt 'DM Sans', sans-serif
}
textarea {
  font: 8pt 'Roboto Mono', monospace
}
a {
  color: #015258;
  text-decoration: underline
}
.title {
  background: transparent;
  background: linear-gradient(top,rgba(38,38,38,0.8),#e6e6e6 25%,#ffffff 38%,#c5c5c5 63%,#f7f7f7 87%,rgba(38,38,38,0.8));
  background: -webkit-linear-gradient(top, rgba(38,38,38,0.5),#e6e6e6 25%,#ffffff 38%,rgba(0,0,0,0.25) 63%,#e6e6e6 87%,rgba(38,38,38,0.4));
  box-shadow: inset 0px 1px 0px rgba(255,255,255,1),0px 1px 3px rgba(0,0,0,0.3);
  color: #888888;
  display: inline-block;
  font: 18pt Urbanist, sans-serif;
  padding: 5px 30px;
  text-align: center;
  text-shadow: 0px -1px 0px rgba(0,0,0,0.4);
  vertical-align: middle
}
.button {
  background: transparent;
  background: linear-gradient(top, rgba(38, 38, 38, 0.8), #e6e6e6 25%, #ffffff 38%, #c5c5c5 63%, #f7f7f7 87%, rgba(38, 38, 38, 0.8));
  background: -webkit-linear-gradient(top, rgba(38, 38, 38, 0.5), #e6e6e6 25%, #ffffff 38%, rgba(0, 0, 0, 0.25)  63%, #e6e6e6 87%, rgba(38, 38, 38, 0.4));
  border: 1px solid #ba6;
  border-color: #7c7c7c;
  border-radius: 5px;
  box-shadow: inset 0px 1px 0px rgba(255,255,255,1),0px 1px 3px rgba(0,0,0,0.3);
  color: #015258;
  cursor: pointer;
  display: inline-block;
  font: 14pt Urbanist, sans-serif;
  font-weight: bold;
  height: 40px;
  padding: 5px 25px;
  text-shadow: 0px -1px 0px rgba(0,0,0,0.4)
}
.button:active{
  -webkit-transform: translateY(2px);
  transform: translateY(2px)
}
.copyright {
  font: 8pt 'DM Sans', sans-serif;
  color: #000000;
  text-align: center
}
`)

	logger.SetDetails(fhctx, zap.InfoLevel, "CSS", nil, nil)
	fhctx.SetBodyString(response.String())
	fhctx.SetContentType("text/css")
	fhctx.SetStatusCode(fasthttp.StatusOK)
}

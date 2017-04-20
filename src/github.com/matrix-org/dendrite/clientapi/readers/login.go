package readers

import (
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/util"
	"net/http"
)

type loginFlows struct {
	Flows []flow `json:"flows"`
}

type flow struct {
	Type   string   `json:"type"`
	Stages []string `json:"stages"`
}

func passwordLogin() loginFlows {
	f := loginFlows{}
	s := flow{"m.login.password", []string{"m.login.password"}}
	f.Flows = append(f.Flows, s)
	return f
}

func Login(req *http.Request) util.JSONResponse {
	if req.Method == "GET" {
		return util.JSONResponse{
			Code: 200,
			JSON: passwordLogin(),
		}
	} else if req.Method == "POST" {
		return util.JSONResponse{
			Code: 403,
			JSON: jsonerror.Forbidden("Not implemented"),
		}
	}
	return util.JSONResponse{
		Code: 405,
		JSON: jsonerror.NotFound("Bad method"),
	}
}

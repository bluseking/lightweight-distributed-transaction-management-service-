package dtmcli

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"
	"github.com/yedf/dtm/common"
)

// M alias
type M = map[string]interface{}

var e2p = common.E2P

// XaGlobalFunc type of xa global function
type XaGlobalFunc func(xa *Xa) error

// XaLocalFunc type of xa local function
type XaLocalFunc func(db *common.DB, xa *Xa) error

// XaClient xa client
type XaClient struct {
	Server      string
	Conf        map[string]string
	CallbackURL string
}

// Xa xa transaction
type Xa struct {
	IDGenerator
	Gid string
}

// GetParams get xa params map
func (x *Xa) GetParams(branchID string) common.MS {
	return common.MS{
		"gid":         x.Gid,
		"trans_type":  "xa",
		"branch_id":   branchID,
		"branch_type": "action",
	}
}

// XaFromReq construct xa info from request
func XaFromReq(c *gin.Context) *Xa {
	return &Xa{
		Gid:         c.Query("gid"),
		IDGenerator: IDGenerator{parentID: c.Query("branch_id")},
	}
}

// NewXaBranchID generate a xa branch id
func (x *Xa) NewXaBranchID() string {
	return x.Gid + "-" + x.NewBranchID()
}

// NewXaClient construct a xa client
func NewXaClient(server string, mysqlConf map[string]string, app *gin.Engine, callbackURL string) *XaClient {
	xa := &XaClient{
		Server:      server,
		Conf:        mysqlConf,
		CallbackURL: callbackURL,
	}
	u, err := url.Parse(callbackURL)
	e2p(err)
	app.POST(u.Path, common.WrapHandler(func(c *gin.Context) (interface{}, error) {
		type CallbackReq struct {
			Gid      string `json:"gid"`
			BranchID string `json:"branch_id"`
			Action   string `json:"action"`
		}
		req := CallbackReq{}
		b, err := c.GetRawData()
		e2p(err)
		common.MustUnmarshal(b, &req)
		tx, my := common.DbAlone(xa.Conf)
		defer my.Close()
		branchID := req.Gid + "-" + req.BranchID
		if req.Action == "commit" {
			tx.Must().Exec(fmt.Sprintf("xa commit '%s'", branchID))
		} else if req.Action == "rollback" {
			tx.Must().Exec(fmt.Sprintf("xa rollback '%s'", branchID))
		} else {
			panic(fmt.Errorf("unknown action: %s", req.Action))
		}
		return M{"dtm_result": "SUCCESS"}, nil
	}))
	return xa
}

// XaLocalTransaction start a xa local transaction
func (xc *XaClient) XaLocalTransaction(c *gin.Context, transFunc XaLocalFunc) (rerr error) {
	defer common.P2E(&rerr)
	xa := XaFromReq(c)
	branchID := xa.NewBranchID()
	xaBranch := xa.Gid + "-" + branchID
	tx, my := common.DbAlone(xc.Conf)
	defer func() { my.Close() }()
	tx.Must().Exec(fmt.Sprintf("XA start '%s'", xaBranch))
	err := transFunc(tx, xa)
	e2p(err)
	resp, err := common.RestyClient.R().
		SetBody(&M{"gid": xa.Gid, "branch_id": branchID, "trans_type": "xa", "status": "prepared", "url": xc.CallbackURL}).
		Post(xc.Server + "/registerXaBranch")
	e2p(err)
	if !strings.Contains(resp.String(), "SUCCESS") {
		e2p(fmt.Errorf("unknown server response: %s", resp.String()))
	}
	tx.Must().Exec(fmt.Sprintf("XA end '%s'", xaBranch))
	tx.Must().Exec(fmt.Sprintf("XA prepare '%s'", xaBranch))
	return nil
}

// XaGlobalTransaction start a xa global transaction
func (xc *XaClient) XaGlobalTransaction(transFunc XaGlobalFunc) (gid string, rerr error) {
	xa := Xa{IDGenerator: IDGenerator{}, Gid: GenGid(xc.Server)}
	gid = xa.Gid
	data := &M{
		"gid":        gid,
		"trans_type": "xa",
	}
	defer func() {
		x := recover()
		if x != nil {
			_, _ = common.RestyClient.R().SetBody(data).Post(xc.Server + "/abort")
			rerr = x.(error)
		}
	}()
	resp, rerr := common.RestyClient.R().SetBody(data).Post(xc.Server + "/prepare")
	e2p(rerr)
	if !strings.Contains(resp.String(), "SUCCESS") {
		panic(fmt.Errorf("unexpected result: %s", resp.String()))
	}
	rerr = transFunc(&xa)
	e2p(rerr)
	resp, rerr = common.RestyClient.R().SetBody(data).Post(xc.Server + "/submit")
	e2p(rerr)
	if !strings.Contains(resp.String(), "SUCCESS") {
		panic(fmt.Errorf("unexpected result: %s", resp.String()))
	}
	return
}

// CallBranch call a xa branch
func (x *Xa) CallBranch(body interface{}, url string) (*resty.Response, error) {
	branchID := x.NewBranchID()
	return common.RestyClient.R().
		SetBody(body).
		SetQueryParams(common.MS{
			"gid":         x.Gid,
			"branch_id":   branchID,
			"trans_type":  "xa",
			"branch_type": "action",
		}).
		Post(url)
}

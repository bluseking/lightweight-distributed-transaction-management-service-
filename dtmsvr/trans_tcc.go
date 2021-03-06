package dtmsvr

import (
	"fmt"
	"strings"

	"github.com/yedf/dtm/common"
)

type transTccProcessor struct {
	*TransGlobal
}

func init() {
	registorProcessorCreator("tcc", func(trans *TransGlobal) transProcessor { return &transTccProcessor{TransGlobal: trans} })
}

func (t *transTccProcessor) GenBranches() []TransBranch {
	return []TransBranch{}
}

func (t *transTccProcessor) ExecBranch(db *common.DB, branch *TransBranch) {
	resp, err := common.RestyClient.R().SetBody(branch.Data).SetHeader("Content-type", "application/json").SetQueryParams(t.getBranchParams(branch)).Post(branch.URL)
	e2p(err)
	body := resp.String()
	if strings.Contains(body, "SUCCESS") {
		t.touch(db, config.TransCronInterval)
		branch.changeStatus(db, "succeed")
	} else if branch.BranchType == "try" && strings.Contains(body, "FAILURE") {
		t.touch(db, config.TransCronInterval)
		branch.changeStatus(db, "failed")
	} else {
		panic(fmt.Errorf("unknown response: %s, will be retried", body))
	}
}

func (t *transTccProcessor) ProcessOnce(db *common.DB, branches []TransBranch) {
	if t.Status == "succeed" || t.Status == "failed" {
		return
	}
	branchType := common.If(t.Status == "submitted", "confirm", "cancel").(string)
	for current := len(branches) - 1; current >= -1; current-- {
		if current == -1 { // 已全部处理完
			t.changeStatus(db, common.If(t.Status == "submitted", "succeed", "failed").(string))
		} else if branches[current].BranchType == branchType {
			t.ExecBranch(db, &branches[current])
		}
	}
}

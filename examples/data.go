package examples

import (
	"io/ioutil"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/yedf/dtm/common"
)

// RunSQLScript 1
func RunSQLScript(mysql map[string]string, script string) {
	conf := map[string]string{}
	common.MustRemarshal(mysql, &conf)
	conf["database"] = ""
	db, con := common.DbAlone(conf)
	defer func() { con.Close() }()
	content, err := ioutil.ReadFile(script)
	if err != nil {
		e2p(err)
	}
	sqls := strings.Split(string(content), ";")
	for _, sql := range sqls {
		s := strings.TrimSpace(sql)
		if s == "" {
			continue
		}
		logrus.Printf("executing: '%s'", s)
		db.Must().Exec(s)
	}
}

// PopulateMysql populate example mysql data
func PopulateMysql() {
	common.InitApp(common.GetProjectDir(), &config)
	config.Mysql["database"] = dbName
	RunSQLScript(config.Mysql, common.GetCurrentCodeDir()+"/examples.sql")
}

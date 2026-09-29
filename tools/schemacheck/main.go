// 对比 GORM 模型与数据库真实表结构，列出缺失的列。
// 用于排查"模型加了字段但 DDL 没跟上"导致的 Error 1054。
//
//	go run ./tools/schemacheck
package main

import (
	"flag"
	"fmt"
	"os"

	"gorm.io/gorm"

	"seckill/pkg/config"
	"seckill/pkg/consts"
	"seckill/pkg/models"
	"seckill/pkg/mysqlx"
)

type target struct {
	dbName string
	model  interface{}
}

func main() {
	cfgPath := flag.String("config", config.DefaultPath, "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Printf("load config failed: %v\n", err)
		os.Exit(1)
	}

	targets := []target{
		{consts.DBUser, &models.User{}},
		{consts.DBProduct, &models.Product{}},
		{consts.DBProduct, &models.SeckillActivity{}},
		{consts.DBStock, &models.Stock{}},
		{consts.DBStock, &models.StockFlow{}},
		{consts.DBOrder, &models.Order{}},
		{consts.DBOrder, &models.MQConsumeLog{}},
	}

	conns := make(map[string]*gorm.DB)
	missing := 0

	for _, t := range targets {
		db, ok := conns[t.dbName]
		if !ok {
			var oerr error
			db, oerr = mysqlx.Open(cfg.MySQL, t.dbName)
			if oerr != nil {
				fmt.Printf("open %s failed: %v\n", t.dbName, oerr)
				os.Exit(1)
			}
			conns[t.dbName] = db
		}

		stmt := &gorm.Statement{DB: db}
		if perr := stmt.Parse(t.model); perr != nil {
			fmt.Printf("parse model failed: %v\n", perr)
			continue
		}

		for _, f := range stmt.Schema.Fields {
			col := f.DBName
			if col == "" {
				continue
			}
			if !db.Migrator().HasColumn(t.model, col) {
				fmt.Printf("MISSING  %s.%-18s column=%s\n", t.dbName, stmt.Schema.Table, col)
				missing++
			}
		}
		fmt.Printf("CHECKED  %s.%s\n", t.dbName, stmt.Schema.Table)
	}

	if missing == 0 {
		fmt.Println("\nALL OK: 模型与表结构一致")
		return
	}
	fmt.Printf("\n共发现 %d 个缺失列\n", missing)
	os.Exit(2)
}

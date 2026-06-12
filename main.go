package main

import (
	"RESTGo/console"
	"RESTGo/stock"
	"fmt"
	"os"
	"strings"
)

func main() {
	defer console.Cleanup()

	if len(os.Args) < 2 {
		fmt.Println("Hello world~!")
		printUsage()
		return
	}

	switch os.Args[1] {
	case "sqlquery":
		handleSQLQuery(os.Args[2:])
	case "stock":
		stock.Handle(os.Args[2:])
	case "py":
		handlePython(os.Args[2:])
	default:
		fmt.Printf("알 수 없는 명령: %s\n", os.Args[1])
		printUsage()
	}
}

func printUsage() {
	fmt.Println("사용법:")
	fmt.Println("  ./RESTGo sqlquery [flags] \"SQL\"")
	fmt.Println("  ./RESTGo stock analyze <종목코드> [일수=250]")
	fmt.Println("  ./RESTGo stock batch [일수=250]")
	fmt.Println("  ./RESTGo py box_chart <종목코드>")
	fmt.Println("  ./RESTGo py box_batch")
	fmt.Println("  ./RESTGo py <스크립트경로> [인수...]")
}

// ─────────────────────────────────────────────
// sqlquery
// ─────────────────────────────────────────────

func handleSQLQuery(args []string) {
	if len(args) == 0 {
		fmt.Println("사용법:")
		fmt.Println("  ./RESTGo sqlquery \"SELECT * FROM 테이블\"")
		fmt.Println("  ./RESTGo sqlquery -db han \"SELECT * FROM 테이블\"")
		fmt.Println("  ./RESTGo sqlquery -db var \"UPDATE ...\"")
		fmt.Println()
		console.PrintAvailableDBs()
		return
	}

	dbName := "key"
	var query string

	if args[0] == "-db" {
		if len(args) < 3 {
			fmt.Println("오류: -db 뒤에 DB명과 쿼리를 입력하세요")
			return
		}
		dbName = args[1]
		query = strings.Join(args[2:], " ")
	} else {
		query = strings.Join(args, " ")
	}

	if strings.TrimSpace(query) == "" {
		fmt.Println("오류: 쿼리를 입력하세요")
		return
	}

	fmt.Printf("[%s] DB: %s\n", console.GenerateTimestampedString(), dbName)
	fmt.Printf("[%s] Query: %s\n", console.GenerateTimestampedString(), query)
	fmt.Println()

	if err := console.RunSQLQuery(dbName, query); err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		os.Exit(1)
	}
}

// ─────────────────────────────────────────────
// py
// ─────────────────────────────────────────────

func handlePython(args []string) {
	if len(args) == 0 {
		fmt.Println("사용법:")
		fmt.Println("  ./RESTGo py box_chart <종목코드>")
		fmt.Println("  ./RESTGo py box_batch")
		fmt.Println("  ./RESTGo py batch_chart")
		fmt.Println("  ./RESTGo py tg_send")
		fmt.Println("  ./RESTGo py <스크립트경로> [인수...]")
		return
	}

	var scriptPath string
	var scriptArgs []string

	switch args[0] {
	case "box_chart":
		scriptPath = "py/analysis/box_chart.py"
		scriptArgs = args[1:]
	case "box_batch":
		scriptPath = "py/analysis/box_chart_batch.py"
		scriptArgs = args[1:]
	case "batch_chart":
		scriptPath = "py/batch/chart_batch.py"
		scriptArgs = args[1:]
	case "tg_send":
		scriptPath = "py/batch/tg_send.py"
		scriptArgs = args[1:]
	default:
		scriptPath = args[0]
		scriptArgs = args[1:]
	}

	if err := console.RunPythonScript(scriptPath, scriptArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Python 실행 오류: %v\n", err)
		os.Exit(1)
	}
}

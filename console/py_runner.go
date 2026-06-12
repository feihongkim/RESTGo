package console

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const PythonBin = "/home/feihong/code/REST/RESTGo/venv/bin/python3"
const ProjectRoot = "/home/feihong/code/REST/RESTGo"

// RunPythonScript 는 프로젝트 venv의 Python으로 스크립트를 실행합니다.
// scriptPath: 프로젝트 루트 기준 상대 경로 또는 절대 경로
func RunPythonScript(scriptPath string, args []string) error {
	if !filepath.IsAbs(scriptPath) {
		scriptPath = filepath.Join(ProjectRoot, scriptPath)
	}

	cmdArgs := append([]string{scriptPath}, args...)
	cmd := exec.Command(PythonBin, cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = append(os.Environ(),
		"PYTHONPATH="+ProjectRoot,
		"MPLBACKEND=Agg",
	)

	fmt.Printf("[py] python %s\n", scriptPath)
	return cmd.Run()
}

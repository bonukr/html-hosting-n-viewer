package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

const webappsDir = "webapps"
const ungroupedLabel = "(미분류)"

// Group webapps 하위 그룹(서브폴더)과 그 안의 HTML 파일 목록.
type Group struct {
	Name    string // 내부 식별자. 미분류는 ""
	Display string // UI 표시명
	Files   []string
}

// safeSegment 경로 구분자·특수 세그먼트를 거부하고 안전한 단일 경로 조각을 반환한다.
func safeSegment(s string) (string, bool) {
	s = filepath.Base(strings.TrimSpace(s))
	if s == "" || s == "." || s == ".." {
		return "", false
	}
	if strings.Contains(s, "/") || strings.Contains(s, "\\") {
		return "", false
	}
	return s, true
}

// resolvePath group/name 조합이 webapps 내부인지 검증하고 전체 경로를 반환한다.
func resolvePath(group, name string) (string, error) {
	if group != "" {
		if _, ok := safeSegment(group); !ok {
			return "", fmt.Errorf("invalid group")
		}
	}
	name, ok := safeSegment(name)
	if !ok {
		return "", fmt.Errorf("invalid name")
	}
	if !strings.EqualFold(filepath.Ext(name), ".html") {
		return "", fmt.Errorf("not html")
	}

	fullPath := filepath.Join(webappsDir, group, name)
	absWebapps, err := filepath.Abs(webappsDir)
	if err != nil {
		return "", err
	}
	absFull, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}
	prefix := absWebapps + string(filepath.Separator)
	if absFull != absWebapps && !strings.HasPrefix(absFull, prefix) {
		return "", fmt.Errorf("path escape")
	}
	return fullPath, nil
}

// listHTMLInDir 디렉토리 내 .html 파일명을 정렬해 반환한다.
func listHTMLInDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".html") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	return files, nil
}

// listGroups webapps 루트를 순회해 그룹별 HTML 목록을 반환한다. 미분류가 항상 첫 번째.
func listGroups() ([]Group, error) {
	entries, err := os.ReadDir(webappsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Group{{Name: "", Display: ungroupedLabel}}, nil
		}
		return nil, err
	}

	var ungrouped []string
	var groupNames []string
	groupFiles := make(map[string][]string)

	for _, e := range entries {
		if e.IsDir() {
			groupNames = append(groupNames, e.Name())
			files, err := listHTMLInDir(filepath.Join(webappsDir, e.Name()))
			if err != nil {
				return nil, err
			}
			groupFiles[e.Name()] = files
		} else if strings.EqualFold(filepath.Ext(e.Name()), ".html") {
			ungrouped = append(ungrouped, e.Name())
		}
	}
	sort.Strings(ungrouped)
	sort.Strings(groupNames)

	groups := []Group{{Name: "", Display: ungroupedLabel, Files: ungrouped}}
	for _, g := range groupNames {
		groups = append(groups, Group{Name: g, Display: g, Files: groupFiles[g]})
	}
	return groups, nil
}

// totalFiles 모든 그룹의 파일 수 합계.
func totalFiles(groups []Group) int {
	n := 0
	for _, g := range groups {
		n += len(g.Files)
	}
	return n
}

// redirectWithMsg 결과 메시지를 쿼리스트링에 담아 메인 페이지로 리다이렉트한다.
func redirectWithMsg(c *gin.Context, kind, msg string) {
	c.Redirect(http.StatusSeeOther, "/?"+kind+"="+url.QueryEscape(msg))
}

func main() {
	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}

	r := gin.Default()

	// html/template 이 href URL 컨텍스트에서 쿼리 파라미터를 자동으로 이스케이프하므로
	// 수동 인코딩을 추가하면 이중 인코딩이 발생한다. 자동 이스케이프에 맡긴다.
	indexTmpl := template.Must(template.New("index").Parse(indexHTML))
	r.SetHTMLTemplate(indexTmpl)

	// 메인 페이지: 그룹별 HTML 목록.
	r.GET("/", func(c *gin.Context) {
		groups, err := listGroups()
		if err != nil {
			c.String(http.StatusInternalServerError, "webapps 디렉토리를 읽을 수 없습니다: %v", err)
			return
		}
		c.HTML(http.StatusOK, "index", gin.H{
			"Groups": groups,
			"Total":  totalFiles(groups),
			"Msg":    c.Query("msg"),
			"Err":    c.Query("err"),
		})
	})

	// 개별 HTML 파일 제공.
	r.GET("/view", func(c *gin.Context) {
		group := c.Query("group")
		name := c.Query("name")
		fullPath, err := resolvePath(group, name)
		if err != nil {
			c.String(http.StatusBadRequest, "잘못된 요청입니다.")
			return
		}
		if _, err := os.Stat(fullPath); err != nil {
			c.String(http.StatusNotFound, "파일을 찾을 수 없습니다.")
			return
		}
		c.File(fullPath)
	})

	// HTML 파일 업로드.
	r.POST("/upload", func(c *gin.Context) {
		fh, err := c.FormFile("file")
		if err != nil {
			redirectWithMsg(c, "err", "업로드할 파일을 선택하세요.")
			return
		}
		name := filepath.Base(fh.Filename)
		if !strings.EqualFold(filepath.Ext(name), ".html") {
			redirectWithMsg(c, "err", "HTML(.html) 파일만 업로드할 수 있습니다.")
			return
		}

		group := c.PostForm("group")
		if group != "" {
			if _, ok := safeSegment(group); !ok {
				redirectWithMsg(c, "err", "잘못된 그룹입니다.")
				return
			}
		}

		dstDir := filepath.Join(webappsDir, group)
		if err := os.MkdirAll(dstDir, 0o755); err != nil {
			redirectWithMsg(c, "err", "디렉토리 생성 실패: "+err.Error())
			return
		}
		dst := filepath.Join(dstDir, name)
		if err := c.SaveUploadedFile(fh, dst); err != nil {
			redirectWithMsg(c, "err", "파일 저장 실패: "+err.Error())
			return
		}
		label := name + " 업로드 완료"
		if group != "" {
			label = "[" + group + "] " + label
		}
		redirectWithMsg(c, "msg", label)
	})

	// HTML 파일 삭제.
	r.POST("/delete", func(c *gin.Context) {
		group := c.PostForm("group")
		name := c.PostForm("name")
		fullPath, err := resolvePath(group, name)
		if err != nil {
			redirectWithMsg(c, "err", "잘못된 파일입니다.")
			return
		}
		if err := os.Remove(fullPath); err != nil {
			if os.IsNotExist(err) {
				redirectWithMsg(c, "err", "파일을 찾을 수 없습니다: "+name)
			} else {
				redirectWithMsg(c, "err", "삭제 실패: "+err.Error())
			}
			return
		}
		redirectWithMsg(c, "msg", name+" 삭제 완료")
	})

	// 파일을 다른 그룹으로 이동.
	r.POST("/move", func(c *gin.Context) {
		group := c.PostForm("group")
		name := c.PostForm("name")
		target := c.PostForm("target")

		if group == target {
			redirectWithMsg(c, "err", "같은 그룹으로는 이동할 수 없습니다.")
			return
		}
		if target != "" {
			if _, ok := safeSegment(target); !ok {
				redirectWithMsg(c, "err", "잘못된 대상 그룹입니다.")
				return
			}
		}

		src, err := resolvePath(group, name)
		if err != nil {
			redirectWithMsg(c, "err", "잘못된 파일입니다.")
			return
		}
		dst, err := resolvePath(target, name)
		if err != nil {
			redirectWithMsg(c, "err", "잘못된 대상 경로입니다.")
			return
		}
		if _, err := os.Stat(src); err != nil {
			redirectWithMsg(c, "err", "파일을 찾을 수 없습니다: "+name)
			return
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			redirectWithMsg(c, "err", "대상 디렉토리 생성 실패: "+err.Error())
			return
		}
		if err := os.Rename(src, dst); err != nil {
			redirectWithMsg(c, "err", "이동 실패: "+err.Error())
			return
		}
		redirectWithMsg(c, "msg", name+" 이동 완료")
	})

	// 그룹(서브폴더) 생성.
	r.POST("/group/create", func(c *gin.Context) {
		name, ok := safeSegment(c.PostForm("name"))
		if !ok {
			redirectWithMsg(c, "err", "유효한 그룹 이름을 입력하세요.")
			return
		}
		path := filepath.Join(webappsDir, name)
		if _, err := os.Stat(path); err == nil {
			redirectWithMsg(c, "err", "이미 존재하는 그룹입니다: "+name)
			return
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			redirectWithMsg(c, "err", "그룹 생성 실패: "+err.Error())
			return
		}
		redirectWithMsg(c, "msg", "그룹 '"+name+"' 생성 완료")
	})

	// 빈 그룹 삭제.
	r.POST("/group/delete", func(c *gin.Context) {
		name := c.PostForm("name")
		if name == "" {
			redirectWithMsg(c, "err", ungroupedLabel+" 그룹은 삭제할 수 없습니다.")
			return
		}
		name, ok := safeSegment(name)
		if !ok {
			redirectWithMsg(c, "err", "잘못된 그룹입니다.")
			return
		}
		path := filepath.Join(webappsDir, name)
		entries, err := os.ReadDir(path)
		if err != nil {
			if os.IsNotExist(err) {
				redirectWithMsg(c, "err", "그룹을 찾을 수 없습니다: "+name)
			} else {
				redirectWithMsg(c, "err", "그룹 확인 실패: "+err.Error())
			}
			return
		}
		if len(entries) > 0 {
			redirectWithMsg(c, "err", "파일이 있는 그룹은 삭제할 수 없습니다: "+name)
			return
		}
		if err := os.Remove(path); err != nil {
			redirectWithMsg(c, "err", "그룹 삭제 실패: "+err.Error())
			return
		}
		redirectWithMsg(c, "msg", "그룹 '"+name+"' 삭제 완료")
	})

	log.Printf("서버 시작: http://localhost%s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("서버 실행 실패: %v", err)
	}
}

const indexHTML = `<!DOCTYPE html>
<html lang="ko">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Webapps 뷰어</title>
<style>
  * { box-sizing: border-box; }
  body { margin: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "Apple SD Gothic Neo", "Malgun Gothic", sans-serif; background: #f9fafb; color: #111827; }
  header { background: #1f2937; color: #fff; padding: 16px 24px; font-size: 18px; font-weight: 600; }
  .wrap { max-width: 860px; margin: 32px auto; padding: 0 20px; }
  h2 { font-size: 14px; text-transform: uppercase; color: #6b7280; letter-spacing: 0.05em; margin: 0 0 16px; }
  h3 { font-size: 16px; margin: 0 0 12px; color: #374151; display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  .group-count { font-size: 13px; color: #6b7280; font-weight: normal; }
  ul { list-style: none; margin: 0; padding: 0; }
  li { margin-bottom: 10px; }
  .group-section { background: #fff; border: 1px solid #e5e7eb; border-radius: 12px; padding: 18px 20px; margin-bottom: 20px; }
  .item-row { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  .item { flex: 1; min-width: 200px; display: flex; align-items: center; justify-content: space-between; padding: 14px 16px; background: #f9fafb; border: 1px solid #e5e7eb; border-radius: 10px; color: #111827; text-decoration: none; font-size: 15px; word-break: break-all; transition: border-color 0.15s, box-shadow 0.15s; }
  .item:hover { border-color: #2563eb; box-shadow: 0 2px 8px rgba(37,99,235,0.12); }
  .item .badge { flex: none; margin-left: 12px; font-size: 12px; color: #2563eb; }
  .empty { color: #9ca3af; padding: 12px 0; font-size: 14px; }
  .btn { border: 0; border-radius: 8px; padding: 8px 12px; font-size: 13px; cursor: pointer; white-space: nowrap; }
  .btn-danger { background: #fee2e2; color: #b91c1c; }
  .btn-danger:hover { background: #fecaca; }
  .btn-primary { background: #2563eb; color: #fff; }
  .btn-primary:hover { background: #1d4ed8; }
  .btn-secondary { background: #e5e7eb; color: #374151; }
  .btn-secondary:hover { background: #d1d5db; }
  .panel { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; background: #fff; border: 1px solid #e5e7eb; border-radius: 10px; padding: 16px 18px; margin-bottom: 24px; }
  .panel input[type=file], .panel input[type=text], .panel select { font-size: 14px; }
  .panel input[type=text] { flex: 1; min-width: 160px; padding: 8px 10px; border: 1px solid #d1d5db; border-radius: 8px; }
  .panel select { padding: 8px 10px; border: 1px solid #d1d5db; border-radius: 8px; }
  .move-form { display: flex; gap: 6px; align-items: center; flex-wrap: wrap; }
  .move-form select { padding: 6px 8px; border: 1px solid #d1d5db; border-radius: 6px; font-size: 13px; }
  .banner { padding: 12px 16px; border-radius: 8px; margin-bottom: 16px; font-size: 14px; }
  .banner.ok { background: #dcfce7; color: #166534; }
  .banner.no { background: #fee2e2; color: #b91c1c; }
</style>
</head>
<body>
<header>Webapps 뷰어</header>
<div class="wrap">
  {{if .Msg}}<div class="banner ok">{{.Msg}}</div>{{end}}
  {{if .Err}}<div class="banner no">{{.Err}}</div>{{end}}

  <h2>그룹 만들기</h2>
  <form class="panel" action="/group/create" method="post">
    <input type="text" name="name" placeholder="새 그룹 이름" required>
    <button type="submit" class="btn btn-primary">그룹 만들기</button>
  </form>

  <h2>HTML 업로드</h2>
  <form class="panel" action="/upload" method="post" enctype="multipart/form-data">
    <input type="file" name="file" accept=".html,text/html" required>
    <select name="group">
      {{range .Groups}}
      <option value="{{.Name}}">{{.Display}}</option>
      {{end}}
    </select>
    <button type="submit" class="btn btn-primary">업로드</button>
  </form>

  <h2>HTML 목록 ({{.Total}})</h2>
  {{range $group := .Groups}}
  <div class="group-section">
    <h3>
      {{$group.Display}}
      <span class="group-count">({{len $group.Files}})</span>
      {{if and (ne $group.Name "") (eq (len $group.Files) 0)}}
      <form action="/group/delete" method="post" style="display:inline" onsubmit="return confirm('{{$group.Display}} 그룹을 삭제할까요?');">
        <input type="hidden" name="name" value="{{$group.Name}}">
        <button type="submit" class="btn btn-danger">그룹 삭제</button>
      </form>
      {{end}}
    </h3>
    {{if $group.Files}}
    <ul>
      {{range $file := $group.Files}}
      <li>
        <div class="item-row">
          <a class="item" href="/view?group={{$group.Name}}&amp;name={{$file}}" target="_blank" rel="noopener">
            <span>{{$file}}</span>
            <span class="badge">새 창으로 열기 ↗</span>
          </a>
          <form class="move-form" action="/move" method="post">
            <input type="hidden" name="group" value="{{$group.Name}}">
            <input type="hidden" name="name" value="{{$file}}">
            <select name="target" required>
              {{range $.Groups}}
              {{if ne .Name $group.Name}}
              <option value="{{.Name}}">{{.Display}}</option>
              {{end}}
              {{end}}
            </select>
            <button type="submit" class="btn btn-secondary">이동</button>
          </form>
          <form action="/delete" method="post" onsubmit="return confirm('{{$file}} 파일을 삭제할까요?');">
            <input type="hidden" name="group" value="{{$group.Name}}">
            <input type="hidden" name="name" value="{{$file}}">
            <button type="submit" class="btn btn-danger">삭제</button>
          </form>
        </div>
      </li>
      {{end}}
    </ul>
    {{else}}
    <div class="empty">이 그룹에 HTML 파일이 없습니다.</div>
    {{end}}
  </div>
  {{end}}
</div>
</body>
</html>`

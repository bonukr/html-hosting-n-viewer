package main

import (
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

// listHTMLFiles webapps 디렉토리의 .html 파일 목록을 정렬해서 반환한다.
func listHTMLFiles() ([]string, error) {
	entries, err := os.ReadDir(webappsDir)
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

	indexTmpl := template.Must(template.New("index").Funcs(template.FuncMap{
		"pathescape": url.PathEscape,
	}).Parse(indexHTML))
	r.SetHTMLTemplate(indexTmpl)

	// 메인 페이지: HTML 목록을 보여준다.
	r.GET("/", func(c *gin.Context) {
		files, err := listHTMLFiles()
		if err != nil {
			c.String(http.StatusInternalServerError, "webapps 디렉토리를 읽을 수 없습니다: %v", err)
			return
		}
		c.HTML(http.StatusOK, "index", gin.H{
			"Files": files,
			"Msg":   c.Query("msg"),
			"Err":   c.Query("err"),
		})
	})

	// 개별 HTML 파일 제공. 경로 탈출을 막기 위해 파일명만 사용한다.
	r.GET("/view/:name", func(c *gin.Context) {
		name := filepath.Base(c.Param("name"))
		if !strings.EqualFold(filepath.Ext(name), ".html") {
			c.String(http.StatusBadRequest, "잘못된 파일입니다.")
			return
		}
		fullPath := filepath.Join(webappsDir, name)
		if _, err := os.Stat(fullPath); err != nil {
			c.String(http.StatusNotFound, "파일을 찾을 수 없습니다: %s", name)
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

		if err := os.MkdirAll(webappsDir, 0o755); err != nil {
			redirectWithMsg(c, "err", "디렉토리 생성 실패: "+err.Error())
			return
		}
		dst := filepath.Join(webappsDir, name)
		if err := c.SaveUploadedFile(fh, dst); err != nil {
			redirectWithMsg(c, "err", "파일 저장 실패: "+err.Error())
			return
		}
		redirectWithMsg(c, "msg", name+" 업로드 완료")
	})

	// HTML 파일 삭제.
	r.POST("/delete", func(c *gin.Context) {
		name := filepath.Base(c.PostForm("name"))
		if name == "" || name == "." || !strings.EqualFold(filepath.Ext(name), ".html") {
			redirectWithMsg(c, "err", "잘못된 파일입니다.")
			return
		}
		fullPath := filepath.Join(webappsDir, name)
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
  .wrap { max-width: 760px; margin: 32px auto; padding: 0 20px; }
  h2 { font-size: 14px; text-transform: uppercase; color: #6b7280; letter-spacing: 0.05em; margin: 0 0 16px; }
  ul { list-style: none; margin: 0; padding: 0; }
  li { margin-bottom: 10px; }
  .item-row { display: flex; align-items: center; gap: 10px; }
  .item { flex: 1; display: flex; align-items: center; justify-content: space-between; padding: 16px 18px; background: #fff; border: 1px solid #e5e7eb; border-radius: 10px; color: #111827; text-decoration: none; font-size: 15px; word-break: break-all; transition: border-color 0.15s, box-shadow 0.15s; }
  .item:hover { border-color: #2563eb; box-shadow: 0 2px 8px rgba(37,99,235,0.12); }
  .item .badge { flex: none; margin-left: 12px; font-size: 12px; color: #2563eb; }
  .empty { color: #9ca3af; padding: 16px; font-size: 15px; background: #fff; border: 1px dashed #d1d5db; border-radius: 10px; }
  .btn { border: 0; border-radius: 8px; padding: 10px 14px; font-size: 14px; cursor: pointer; }
  .btn-danger { background: #fee2e2; color: #b91c1c; }
  .btn-danger:hover { background: #fecaca; }
  .btn-primary { background: #2563eb; color: #fff; }
  .btn-primary:hover { background: #1d4ed8; }
  .upload { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; background: #fff; border: 1px solid #e5e7eb; border-radius: 10px; padding: 16px 18px; margin-bottom: 24px; }
  .upload input[type=file] { flex: 1; min-width: 200px; font-size: 14px; }
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

  <h2>HTML 업로드</h2>
  <form class="upload" action="/upload" method="post" enctype="multipart/form-data">
    <input type="file" name="file" accept=".html,text/html" required>
    <button type="submit" class="btn btn-primary">업로드</button>
  </form>

  <h2>HTML 목록 ({{len .Files}})</h2>
  {{if .Files}}
  <ul>
    {{range .Files}}
    <li>
      <div class="item-row">
        <a class="item" href="/view/{{. | pathescape}}" target="_blank" rel="noopener">
          <span>{{.}}</span>
          <span class="badge">새 창으로 열기 ↗</span>
        </a>
        <form action="/delete" method="post" onsubmit="return confirm('{{.}} 파일을 삭제할까요?');">
          <input type="hidden" name="name" value="{{.}}">
          <button type="submit" class="btn btn-danger">삭제</button>
        </form>
      </div>
    </li>
    {{end}}
  </ul>
  {{else}}
  <div class="empty">webapps 디렉토리에 HTML 파일이 없습니다.</div>
  {{end}}
</div>
</body>
</html>`

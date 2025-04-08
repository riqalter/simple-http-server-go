package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FileInfo represents information about a file or directory
type FileInfo struct {
	Name        string
	IsDir       bool
	Size        int64
	ModTime     time.Time
	Path        string
	IsImage     bool
	IsVideo     bool
	ContentType string
	Extension   string // Added to help with icon selection
}

// TemplateData represents the data passed to the HTML template
type TemplateData struct {
	Title       string
	CurrentPath string
	ParentPath  string
	Files       []FileInfo
}

func main() {
	// verbose
	verbose := flag.Bool("v", false, "should everything be logged to stdout")
	// directory nya
	dir := flag.String("dir", ".", "the directory of static file to host")
	// port nya
	port := flag.Int("port", 9000, "port to serve on")
	flag.Parse()

	// working directory
	absDir, err := filepath.Abs(*dir)
	if err != nil {
		log.Fatalf("Could not determine the absolute path of directory %s", *dir)
	}

	// Verbose logging
	if *verbose {
		log.Printf("Verbose mode enabled")
		log.Printf("Serving directory: %s", absDir)
		log.Printf("Listening on port: %d", *port)
	}

	// Create a file server handler for static files
	fileServer := http.FileServer(http.Dir(absDir))

	// Create custom handler for directory listing
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handleRequest(w, r, absDir, fileServer, *verbose)
	})

	// Start server
	fmt.Printf("Serving directory %s on HTTP port: %d\n", absDir, *port)
	err = http.ListenAndServe(fmt.Sprintf(":%d", *port), nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
		os.Exit(1)
	}
}

func handleRequest(w http.ResponseWriter, r *http.Request, rootDir string, fileServer http.Handler, verbose bool) {
	start := time.Now()
	if verbose {
		log.Printf("Received request: %s %s", r.Method, r.URL.Path)
	}

	// Handle thumbnail requests
	if strings.HasPrefix(r.URL.Path, "/_thumbnail/") {
		serveThumbnail(w, r, rootDir)
		if verbose {
			log.Printf("Served thumbnail request: %s, Duration: %s", r.URL.Path, time.Since(start))
		}
		return
	}

	// Get the absolute path of the requested file/directory
	requestPath := filepath.Join(rootDir, filepath.Clean(r.URL.Path))
	relPath, _ := filepath.Rel(rootDir, requestPath)
	if relPath == "." {
		relPath = ""
	}

	// Check if path exists
	fileInfo, err := os.Stat(requestPath)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// If it's a directory, render our custom directory listing
	if fileInfo.IsDir() {
		renderDirectoryListing(w, r, requestPath, rootDir, relPath, verbose)
		return
	}

	// Check if it's a media file that we want to display in our media viewer
	contentType := getContentType(requestPath)
	isMedia := strings.HasPrefix(contentType, "image/") || strings.HasPrefix(contentType, "video/")

	// If viewing parameter is set, show the media viewer
	if r.URL.Query().Get("view") == "media" && isMedia {
		renderMediaViewer(w, r, requestPath, rootDir, relPath, contentType)
		return
	}

	// Otherwise serve the file directly
	fileServer.ServeHTTP(w, r)

	if verbose {
		log.Printf("Served request: %s %s, Duration: %s", r.Method, r.URL.Path, time.Since(start))
	}
}

// New function to handle thumbnail requests
func serveThumbnail(w http.ResponseWriter, r *http.Request, rootDir string) {
	// Extract the actual file path from the thumbnail request
	filePath := strings.TrimPrefix(r.URL.Path, "/_thumbnail")
	fullPath := filepath.Join(rootDir, filePath)

	// Check if the file exists
	_, err := os.Stat(fullPath)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// Set appropriate headers
	contentType := getContentType(fullPath)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400") // Cache thumbnails for 24 hours

	// Serve the file
	http.ServeFile(w, r, fullPath)
}

func renderDirectoryListing(w http.ResponseWriter, r *http.Request, dirPath, rootDir, relPath string, verbose bool) {
	// Read directory contents
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Process entries
	var files []FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		entryPath := filepath.Join(relPath, entry.Name())
		if entryPath == "" {
			entryPath = entry.Name()
		}

		absPath := filepath.Join(dirPath, entry.Name())
		contentType := ""
		isImage := false
		isVideo := false
		extension := ""

		if !entry.IsDir() {
			extension = strings.ToLower(filepath.Ext(entry.Name()))
			contentType = getContentType(absPath)
			isImage = strings.HasPrefix(contentType, "image/")
			isVideo = strings.HasPrefix(contentType, "video/")
		}

		files = append(files, FileInfo{
			Name:        entry.Name(),
			IsDir:       entry.IsDir(),
			Size:        info.Size(),
			ModTime:     info.ModTime(),
			Path:        "/" + entryPath,
			IsImage:     isImage,
			IsVideo:     isVideo,
			ContentType: contentType,
			Extension:   extension,
		})
	}

	// Sort entries: directories first, then files alphabetically
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	// Calculate parent directory path
	parentPath := ""
	if relPath != "" {
		parentPath = "/" + filepath.Dir(relPath)
		if parentPath == "/." {
			parentPath = "/"
		}
	}

	// Prepare template data
	title := "Index of /" + relPath
	if relPath == "" {
		title = "Index of /"
	}

	data := TemplateData{
		Title:       title,
		CurrentPath: "/" + relPath,
		ParentPath:  parentPath,
		Files:       files,
	}

	// Render the template
	renderTemplate(w, data)
}

func renderMediaViewer(w http.ResponseWriter, r *http.Request, filePath, rootDir, relPath, contentType string) {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// Get file info
	fileName := filepath.Base(filePath)
	isVideo := strings.HasPrefix(contentType, "video/")
	isImage := strings.HasPrefix(contentType, "image/")

	// Get parent directory
	parentDir := filepath.Dir(relPath)
	if parentDir == "." {
		parentDir = ""
	}
	parentPath := "/" + parentDir

	// Get the path for direct file access
	filePath = "/" + relPath

	mediaTemplate := `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{ .Name }}</title>
    <style>
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            margin: 0;
            padding: 0;
            color: #333;
            background-color: #f5f5f5;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
        }
        header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 10px 20px;
            background-color: #fff;
            box-shadow: 0 2px 5px rgba(0,0,0,0.1);
            margin-bottom: 20px;
        }
        h1 {
            font-size: 24px;
            margin: 0;
        }
        .media-container {
            background-color: #000;
            display: flex;
            justify-content: center;
            align-items: center;
            margin-bottom: 20px;
            position: relative;
            border-radius: 4px;
            overflow: hidden;
        }
        img {
            max-width: 100%;
            max-height: 80vh;
            display: block;
        }
        video {
            max-width: 100%;
            max-height: 80vh;
        }
        .controls {
            display: flex;
            justify-content: space-between;
            padding: 10px;
            background-color: #fff;
            border-radius: 4px;
            box-shadow: 0 1px 3px rgba(0,0,0,0.1);
        }
        a.button {
            display: inline-block;
            padding: 8px 16px;
            background-color: #4CAF50;
            color: white;
            text-decoration: none;
            border-radius: 4px;
            margin-right: 10px;
            transition: background-color 0.3s;
        }
        a.button:hover {
            background-color: #45a049;
        }
        .popup-button {
            background-color: #2196F3;
        }
        .popup-button:hover {
            background-color: #0b7dda;
        }
        .download-button {
            background-color: #ff9800;
        }
        .download-button:hover {
            background-color: #e68a00;
        }
        .metadata {
            background-color: #fff;
            padding: 15px;
            border-radius: 4px;
            box-shadow: 0 1px 3px rgba(0,0,0,0.1);
        }
        .metadata p {
            margin: 5px 0;
            font-size: 14px;
        }
    </style>
</head>
<body>
    <header>
        <h1>{{ .Name }}</h1>
        <div>
            <a href="{{ .ParentPath }}" class="button">Back to directory</a>
        </div>
    </header>
    <div class="container">
        <div class="media-container">
            {{ if .IsImage }}
            <img src="{{ .FilePath }}" alt="{{ .Name }}">
            {{ else if .IsVideo }}
            <video id="videoPlayer" controls>
                <source src="{{ .FilePath }}" type="{{ .ContentType }}">
                Your browser does not support the video tag.
            </video>
            {{ end }}
        </div>
        
        <div class="controls">
            <div>
                {{ if .IsVideo }}
                <button id="popupBtn" class="button popup-button">Pop-out Player</button>
                {{ end }}
                <a href="{{ .FilePath }}" download class="button download-button">Download</a>
            </div>
        </div>
        
        <div class="metadata">
            <p><strong>File name:</strong> {{ .Name }}</p>
            <p><strong>File size:</strong> {{ .Size }}</p>
            <p><strong>Last modified:</strong> {{ .ModTime }}</p>
            <p><strong>Content type:</strong> {{ .ContentType }}</p>
        </div>
    </div>
    
    {{ if .IsVideo }}
    <script>
        document.getElementById('popupBtn').addEventListener('click', function() {
            const videoSrc = '{{ .FilePath }}';
            const contentType = '{{ .ContentType }}';
            const width = 800;
            const height = 500;
            
            const left = (screen.width - width) / 2;
            const top = (screen.height - height) / 2;
            
            const popupWindow = window.open('', '{{ .Name }}', 'width='+width+',height='+height+',top='+top+',left='+left);
            
            const popupContent = '<!DOCTYPE html>' +
                '<html lang="en">' +
                '<head>' +
                '    <meta charset="UTF-8">' +
                '    <title>{{ .Name }}</title>' +
                '    <style>' +
                '        body { margin: 0; background-color: #000; overflow: hidden; }' +
                '        video { width: 100%; height: 100vh; }' +
                '    </style>' +
                '</head>' +
                '<body>' +
                '    <video id="popupVideo" controls autoplay>' +
                '        <source src="' + videoSrc + '" type="' + contentType + '">' +
                '        Your browser does not support the video tag.' +
                '    </video>' +
                '    <script>' +
                '        const video = document.getElementById("popupVideo");' +
                '        const mainVideo = window.opener.document.getElementById("videoPlayer");' +
                '        if (mainVideo) {' +
                '            video.currentTime = mainVideo.currentTime;' +
                '        }' +
                '        window.onbeforeunload = function() {' +
                '            if (mainVideo && !mainVideo.paused) {' +
                '                mainVideo.pause();' +
                '            }' +
                '        };' +
                '    <\/script>' +
                '</body>' +
                '</html>';
            
            popupWindow.document.write(popupContent);
            popupWindow.document.close();
        });
    </script>
    {{ end }}
</body>
</html>`

	tmpl, err := template.New("mediaViewer").Parse(mediaTemplate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Format file size
	fileSize := formatFileSize(fileInfo.Size())

	data := struct {
		Name        string
		FilePath    string
		ParentPath  string
		IsImage     bool
		IsVideo     bool
		ContentType string
		Size        string
		ModTime     string
	}{
		Name:        fileName,
		FilePath:    filePath,
		ParentPath:  parentPath,
		IsImage:     isImage,
		IsVideo:     isVideo,
		ContentType: contentType,
		Size:        fileSize,
		ModTime:     fileInfo.ModTime().Format("Jan 02, 2006 15:04:05"),
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func renderTemplate(w http.ResponseWriter, data TemplateData) {
	// HTML template for directory listing
	const htmlTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{ .Title }}</title>
    <style>
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            margin: 0;
            padding: 0;
            background-color: #f5f5f5;
            color: #333;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
        }
        header {
            background-color: #fff;
            padding: 20px;
            box-shadow: 0 2px 5px rgba(0,0,0,0.1);
            margin-bottom: 20px;
            border-radius: 4px;
        }
        h1 {
            margin: 0;
            color: #2c3e50;
            font-weight: 500;
        }
        .path-nav {
            font-size: 14px;
            color: #7f8c8d;
            margin-top: 10px;
        }
        .files-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
            gap: 15px;
        }
        .file-card {
            background-color: #fff;
            border-radius: 4px;
            overflow: hidden;
            box-shadow: 0 1px 3px rgba(0,0,0,0.1);
            transition: transform 0.2s, box-shadow 0.2s;
        }
        .file-card:hover {
            transform: translateY(-5px);
            box-shadow: 0 5px 15px rgba(0,0,0,0.1);
        }
        .thumbnail {
            height: 150px;
            background-color: #f8f9fa;
            position: relative;
            overflow: hidden;
            display: flex;
            justify-content: center;
            align-items: center;
            background-size: cover;
            background-position: center;
            background-repeat: no-repeat;
        }
        .thumbnail img {
            max-width: 100%;
            max-height: 100%;
            object-fit: contain;
        }
        .directory .thumbnail {
            background-color: #e3f2fd;
        }
        .file-icon {
            font-size: 50px;
            color: #7f8c8d;
        }
        .directory .file-icon {
            color: #2196F3;
        }
        .file-info {
            padding: 10px;
        }
        .file-name {
            font-weight: 500;
            margin-bottom: 5px;
            word-break: break-word;
            font-size: 14px;
        }
        .file-meta {
            font-size: 12px;
            color: #7f8c8d;
        }
        a {
            text-decoration: none;
            color: inherit;
        }
        .video-thumbnail {
            position: relative;
        }
        .play-icon {
            position: absolute;
            top: 50%;
            left: 50%;
            transform: translate(-50%, -50%);
            width: 50px;
            height: 50px;
            background-color: rgba(0, 0, 0, 0.7);
            border-radius: 50%;
            display: flex;
            justify-content: center;
            align-items: center;
        }
        .play-icon:before {
            content: '';
            width: 0;
            height: 0;
            border-top: 10px solid transparent;
            border-left: 20px solid white;
            border-bottom: 10px solid transparent;
            margin-left: 5px;
        }
        .file-thumbnail {
            font-size: 50px;
            color: #7f8c8d;
            text-align: center;
            line-height: 150px;
        }
        /* File type icons */
        .icon-pdf { color: #e74c3c; }
        .icon-doc, .icon-docx { color: #3498db; }
        .icon-xls, .icon-xlsx { color: #2ecc71; }
        .icon-txt { color: #95a5a6; }
        .icon-zip, .icon-rar, .icon-7z { color: #f39c12; }
        .icon-mp3, .icon-wav, .icon-ogg { color: #9b59b6; }
        /* Media query for mobile devices */
        @media (max-width: 768px) {
            .files-grid {
                grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
            }
            .thumbnail {
                height: 120px;
            }
        }
        /* Icon styles */
        .icon {
            width: 60px;
            height: 60px;
            display: inline-flex;
            justify-content: center;
            align-items: center;
        }
        .directory-icon {
            color: #2196F3;
            font-size: 60px;
        }
        .file-icon-generic {
            color: #7f8c8d;
            font-size: 50px;
        }
        /* Parent directory arrow */
        .back-link {
            display: flex;
            align-items: center;
            padding: 10px;
            background-color: #fff;
            border-radius: 4px;
            margin-bottom: 15px;
            box-shadow: 0 1px 3px rgba(0,0,0,0.1);
            font-weight: 500;
        }
        .back-arrow {
            margin-right: 10px;
            font-size: 20px;
        }
        .lazy-load {
            opacity: 0;
            transition: opacity 0.3s ease-in;
        }
        .lazy-loaded {
            opacity: 1;
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>{{ .Title }}</h1>
            <div class="path-nav">{{ .CurrentPath }}</div>
        </header>
        
        {{ if .ParentPath }}
        <a href="{{ .ParentPath }}" class="back-link">
            <span class="back-arrow">←</span> Parent Directory
        </a>
        {{ end }}
        
        <div class="files-grid">
            {{ range .Files }}
            <div class="file-card {{ if .IsDir }}directory{{ end }}">
                {{ if .IsDir }}
                <a href="{{ .Path }}">
                    <div class="thumbnail">
                        <div class="icon directory-icon">📁</div>
                    </div>
                    <div class="file-info">
                        <div class="file-name">{{ .Name }}</div>
                        <div class="file-meta">Directory</div>
                    </div>
                </a>
                {{ else if .IsImage }}
                <a href="{{ .Path }}?view=media">
                    <div class="thumbnail">
                        <img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=" data-src="/_thumbnail{{ .Path }}" alt="{{ .Name }}" class="lazy-load">
                    </div>
                    <div class="file-info">
                        <div class="file-name">{{ .Name }}</div>
                        <div class="file-meta">Image · {{ formatSize .Size }}</div>
                    </div>
                </a>
                {{ else if .IsVideo }}
                <a href="{{ .Path }}?view=media">
                    <div class="thumbnail video-thumbnail">
                        <div class="file-thumbnail icon-video">🎬</div>
                        <div class="play-icon"></div>
                    </div>
                    <div class="file-info">
                        <div class="file-name">{{ .Name }}</div>
                        <div class="file-meta">Video · {{ formatSize .Size }}</div>
                    </div>
                </a>
                {{ else }}
                <a href="{{ .Path }}">
                    <div class="thumbnail">
                        <div class="file-thumbnail {{ getFileIconClass .Extension }}">{{ getFileIcon .Extension }}</div>
                    </div>
                    <div class="file-info">
                        <div class="file-name">{{ .Name }}</div>
                        <div class="file-meta">{{ formatSize .Size }} · {{ formatDate .ModTime }}</div>
                    </div>
                </a>
                {{ end }}
            </div>
            {{ end }}
        </div>
    </div>
    
    <script>
    // Lazy loading implementation
    document.addEventListener('DOMContentLoaded', function() {
        let lazyImages = [].slice.call(document.querySelectorAll('.lazy-load'));
        
        if ('IntersectionObserver' in window) {
            let lazyImageObserver = new IntersectionObserver(function(entries, observer) {
                entries.forEach(function(entry) {
                    if (entry.isIntersecting) {
                        let lazyImage = entry.target;
                        lazyImage.src = lazyImage.dataset.src;
                        lazyImage.classList.add('lazy-loaded');
                        lazyImageObserver.unobserve(lazyImage);
                        
                        // Error handling
                        lazyImage.onerror = function() {
                            this.src = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=';
                            this.parentNode.style.backgroundColor = '#f8f9fa';
                            this.parentNode.innerHTML = '<div class="file-thumbnail">🖼️</div>';
                        };
                        
                        // Add load event to handle successful loads
                        lazyImage.onload = function() {
                            this.classList.add('lazy-loaded');
                        };
                    }
                });
            });
            
            lazyImages.forEach(function(lazyImage) {
                lazyImageObserver.observe(lazyImage);
            });
        } else {
            // Fallback for browsers without IntersectionObserver
            let active = false;
            
            const lazyLoad = function() {
                if (active === false) {
                    active = true;
                    
                    setTimeout(function() {
                        lazyImages.forEach(function(lazyImage) {
                            if ((lazyImage.getBoundingClientRect().top <= window.innerHeight && lazyImage.getBoundingClientRect().bottom >= 0) && getComputedStyle(lazyImage).display !== 'none') {
                                lazyImage.src = lazyImage.dataset.src;
                                lazyImage.classList.add('lazy-loaded');
                                
                                lazyImage.onerror = function() {
                                    this.src = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=';
                                    this.parentNode.style.backgroundColor = '#f8f9fa';
                                    this.parentNode.innerHTML = '<div class="file-thumbnail">🖼️</div>';
                                };
                                
                                lazyImages = lazyImages.filter(function(image) {
                                    return image !== lazyImage;
                                });
                                
                                if (lazyImages.length === 0) {
                                    document.removeEventListener('scroll', lazyLoad);
                                    window.removeEventListener('resize', lazyLoad);
                                    window.removeEventListener('orientationchange', lazyLoad);
                                }
                            }
                        });
                        
                        active = false;
                    }, 200);
                }
            };
            
            document.addEventListener('scroll', lazyLoad);
            window.addEventListener('resize', lazyLoad);
            window.addEventListener('orientationchange', lazyLoad);
            lazyLoad();
        }
    });
    </script>
</body>
</html>
`

	// Create template with custom functions
	funcMap := template.FuncMap{
		"formatSize": formatFileSize,
		"formatDate": func(t time.Time) string {
			return t.Format("Jan 02, 2006")
		},
		"getFileIcon": func(ext string) string {
			switch ext {
			case ".pdf":
				return "📄"
			case ".doc", ".docx":
				return "📝"
			case ".xls", ".xlsx":
				return "📊"
			case ".txt":
				return "📄"
			case ".zip", ".rar", ".7z":
				return "🗜️"
			case ".mp3", ".wav", ".ogg", ".flac":
				return "🎵"
			case ".exe", ".msi":
				return "⚙️"
			case ".js", ".py", ".php", ".html", ".css", ".go", ".java":
				return "💻"
			default:
				return "📄"
			}
		},
		"getFileIconClass": func(ext string) string {
			switch ext {
			case ".pdf":
				return "icon-pdf"
			case ".doc", ".docx":
				return "icon-doc"
			case ".xls", ".xlsx":
				return "icon-xls"
			case ".txt":
				return "icon-txt"
			case ".zip", ".rar", ".7z":
				return "icon-zip"
			case ".mp3", ".wav", ".ogg":
				return "icon-mp3"
			default:
				return "icon-generic"
			}
		},
	}

	tmpl, err := template.New("directoryListing").Funcs(funcMap).Parse(htmlTemplate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Helper functions
func formatFileSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}

func getContentType(path string) string {
	ext := filepath.Ext(path)
	contentType := mime.TypeByExtension(ext)
	
	if contentType == "" {
		// Try to detect content type for common media files
		switch strings.ToLower(ext) {
		case ".mp4":
			return "video/mp4"
		case ".webm":
			return "video/webm"
		case ".ogg":
			return "video/ogg"
		case ".m4v":
			return "video/x-m4v"
		case ".mkv":
			return "video/webm"
		case ".mov":
			return "video/quicktime"
		case ".mp3":
			return "audio/mpeg"
		case ".jpg", ".jpeg":
			return "image/jpeg"
		case ".png":
			return "image/png"
		case ".gif":
			return "image/gif"
		case ".webp":
			return "image/webp"
		default:
			return "application/octet-stream"
		}
	}
	
	return contentType
}

// loggingHandler wraps an http.Handler to log requests
func loggingHandler(handler http.Handler, verbose bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		if verbose {
			log.Printf("Received request: %s %s", r.Method, r.URL.Path)
		}
		// Create a ResponseWriter wrapper to capture the status code
		lrw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		handler.ServeHTTP(lrw, r)
		if verbose {
			log.Printf("Served request: %s %s, Status: %d, Duration: %s", r.Method, r.URL.Path, lrw.statusCode, time.Since(start))
		}
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

// output status code
func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}
package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"
)

func main() {
	specPath := flag.String("spec", "", "path to openapi.yaml")
	goOut := flag.String("go-out", "", "output directory for Go SDK")
	pythonOut := flag.String("python-out", "", "output directory for Python SDK")
	tsOut := flag.String("ts-out", "", "output directory for TypeScript SDK")
	module := flag.String("module", "", "Go module path for the generated SDK (e.g. github.com/myorg/myproject-sdk)")
	apiPrefix := flag.String("api-prefix", "", "API path prefix (e.g. /api/rh-trex-ai/v1)")
	projectName := flag.String("project", "", "project name for SDK branding (e.g. rh-trex)")
	flag.Parse()

	if *specPath == "" {
		log.Fatal("--spec is required")
	}
	if *goOut == "" && *pythonOut == "" && *tsOut == "" {
		log.Fatal("at least one of --go-out, --python-out, or --ts-out is required")
	}

	if *apiPrefix == "" {
		*apiPrefix = inferAPIPrefix(*specPath)
	}

	if *projectName == "" {
		parts := strings.Split(*apiPrefix, "/")
		for _, p := range parts {
			if p != "" && p != "api" && p != "v1" && p != "v2" {
				*projectName = p
				break
			}
		}
		if *projectName == "" {
			*projectName = "sdk"
		}
	}

	spec, err := parseSpec(*specPath, *apiPrefix)
	if err != nil {
		log.Fatalf("parse spec: %v", err)
	}

	spec.APIPrefix = *apiPrefix
	spec.Module = *module
	spec.Project = *projectName

	specHash, err := computeSpecHash(*specPath)
	if err != nil {
		log.Fatalf("compute spec hash: %v", err)
	}

	header := GeneratedHeader{
		SpecPath:  *specPath,
		SpecHash:  specHash,
		Timestamp: generationTimestamp(),
	}

	fmt.Printf("Parsed %d resources from %s\n", len(spec.Resources), *specPath)
	for _, r := range spec.Resources {
		fmt.Printf("  %s (%s): %d fields, delete=%v, patch=%v, actions=%v\n",
			r.Name, r.PathSegment, len(r.Fields), r.HasDelete, r.HasPatch, r.Actions)
	}

	if *goOut != "" {
		if err := generateGo(spec, *goOut, header); err != nil {
			log.Fatalf("generate Go: %v", err)
		}
		fmt.Printf("Go SDK generated in %s\n", *goOut)
	}

	if *pythonOut != "" {
		if err := generatePython(spec, *pythonOut, header); err != nil {
			log.Fatalf("generate Python: %v", err)
		}
		fmt.Printf("Python SDK generated in %s\n", *pythonOut)
	}

	if *tsOut != "" {
		if err := generateTypeScript(spec, *tsOut, header); err != nil {
			log.Fatalf("generate TypeScript: %v", err)
		}
		fmt.Printf("TypeScript SDK generated in %s\n", *tsOut)
	}
}

type GeneratedHeader struct {
	SpecPath  string
	SpecHash  string
	Timestamp string
}

type templateData struct {
	Header   GeneratedHeader
	Resource Resource
	Spec     *Spec
}

func generateGo(spec *Spec, outDir string, header GeneratedHeader) error {
	typesDir := filepath.Join(outDir, "types")
	clientDir := filepath.Join(outDir, "client")
	if err := os.MkdirAll(typesDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(clientDir, 0755); err != nil {
		return err
	}

	tmplDir := filepath.Join(getTemplateDir(), "go")

	baseTmpl, err := loadTemplate(filepath.Join(tmplDir, "base.go.tmpl"))
	if err != nil {
		return fmt.Errorf("load base template: %w", err)
	}
	if err := executeTemplate(baseTmpl, filepath.Join(typesDir, "base.go"), templateData{Header: header, Spec: spec}); err != nil {
		return fmt.Errorf("execute base template: %w", err)
	}

	typesTmpl, err := loadTemplate(filepath.Join(tmplDir, "types.go.tmpl"))
	if err != nil {
		return fmt.Errorf("load types template: %w", err)
	}

	clientTmpl, err := loadTemplate(filepath.Join(tmplDir, "client.go.tmpl"))
	if err != nil {
		return fmt.Errorf("load client template: %w", err)
	}

	for _, r := range spec.Resources {
		data := templateData{Header: header, Resource: r, Spec: spec}
		fileName := toSnakeCase(r.Name) + ".go"

		if err := executeTemplate(typesTmpl, filepath.Join(typesDir, fileName), data); err != nil {
			return fmt.Errorf("execute types template for %s: %w", r.Name, err)
		}

		apiFileName := toSnakeCase(r.Name) + "_api.go"
		if err := executeTemplate(clientTmpl, filepath.Join(clientDir, apiFileName), data); err != nil {
			return fmt.Errorf("execute client template for %s: %w", r.Name, err)
		}
	}

	iteratorTmpl, err := loadTemplate(filepath.Join(tmplDir, "iterator.go.tmpl"))
	if err != nil {
		return fmt.Errorf("load iterator template: %w", err)
	}
	if err := executeTemplate(iteratorTmpl, filepath.Join(clientDir, "iterator.go"), templateData{Header: header, Spec: spec}); err != nil {
		return fmt.Errorf("execute iterator template: %w", err)
	}

	listOptsTmpl, err := loadTemplate(filepath.Join(tmplDir, "list_options.go.tmpl"))
	if err != nil {
		return fmt.Errorf("load list_options template: %w", err)
	}
	if err := executeTemplate(listOptsTmpl, filepath.Join(typesDir, "list_options.go"), templateData{Header: header, Spec: spec}); err != nil {
		return fmt.Errorf("execute list_options template: %w", err)
	}

	httpClientTmpl, err := loadTemplate(filepath.Join(tmplDir, "http_client.go.tmpl"))
	if err != nil {
		return fmt.Errorf("load http_client template: %w", err)
	}
	if err := executeTemplate(httpClientTmpl, filepath.Join(clientDir, "client.go"), templateData{Header: header, Spec: spec}); err != nil {
		return fmt.Errorf("execute http_client template: %w", err)
	}

	return nil
}

func generatePython(spec *Spec, outDir string, header GeneratedHeader) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}

	tmplDir := filepath.Join(getTemplateDir(), "python")

	baseTmpl, err := loadTemplate(filepath.Join(tmplDir, "base.py.tmpl"))
	if err != nil {
		return fmt.Errorf("load base template: %w", err)
	}
	if err := executeTemplate(baseTmpl, filepath.Join(outDir, "_base.py"), templateData{Header: header, Spec: spec}); err != nil {
		return fmt.Errorf("execute base template: %w", err)
	}

	typesTmpl, err := loadTemplate(filepath.Join(tmplDir, "types.py.tmpl"))
	if err != nil {
		return fmt.Errorf("load types template: %w", err)
	}

	clientTmpl, err := loadTemplate(filepath.Join(tmplDir, "client.py.tmpl"))
	if err != nil {
		return fmt.Errorf("load client template: %w", err)
	}

	for _, r := range spec.Resources {
		data := templateData{Header: header, Resource: r, Spec: spec}
		fileName := toSnakeCase(r.Name) + ".py"
		if err := executeTemplate(typesTmpl, filepath.Join(outDir, fileName), data); err != nil {
			return fmt.Errorf("execute types template for %s: %w", r.Name, err)
		}

		apiFileName := "_" + toSnakeCase(r.Name) + "_api.py"
		if err := executeTemplate(clientTmpl, filepath.Join(outDir, apiFileName), data); err != nil {
			return fmt.Errorf("execute client template for %s: %w", r.Name, err)
		}
	}

	iteratorTmpl, err := loadTemplate(filepath.Join(tmplDir, "iterator.py.tmpl"))
	if err != nil {
		return fmt.Errorf("load iterator template: %w", err)
	}
	if err := executeTemplate(iteratorTmpl, filepath.Join(outDir, "_iterator.py"), templateData{Header: header, Spec: spec}); err != nil {
		return fmt.Errorf("execute iterator template: %w", err)
	}

	httpClientTmpl, err := loadTemplate(filepath.Join(tmplDir, "http_client.py.tmpl"))
	if err != nil {
		return fmt.Errorf("load http_client template: %w", err)
	}
	if err := executeTemplate(httpClientTmpl, filepath.Join(outDir, "client.py"), templateData{Header: header, Spec: spec}); err != nil {
		return fmt.Errorf("execute http_client template: %w", err)
	}

	initTmpl, err := loadTemplate(filepath.Join(tmplDir, "__init__.py.tmpl"))
	if err != nil {
		return fmt.Errorf("load __init__.py template: %w", err)
	}
	if err := executeTemplate(initTmpl, filepath.Join(outDir, "__init__.py"), templateData{Header: header, Spec: spec}); err != nil {
		return fmt.Errorf("execute __init__.py template: %w", err)
	}

	return nil
}

func generateTypeScript(spec *Spec, outDir string, header GeneratedHeader) error {
	srcDir := filepath.Join(outDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return err
	}

	tmplDir := filepath.Join(getTemplateDir(), "ts")

	baseTmpl, err := loadTemplate(filepath.Join(tmplDir, "base.ts.tmpl"))
	if err != nil {
		return fmt.Errorf("load base template: %w", err)
	}
	if err := executeTemplate(baseTmpl, filepath.Join(srcDir, "base.ts"), templateData{Header: header, Spec: spec}); err != nil {
		return fmt.Errorf("execute base template: %w", err)
	}

	typesTmpl, err := loadTemplate(filepath.Join(tmplDir, "types.ts.tmpl"))
	if err != nil {
		return fmt.Errorf("load types template: %w", err)
	}

	clientTmpl, err := loadTemplate(filepath.Join(tmplDir, "client.ts.tmpl"))
	if err != nil {
		return fmt.Errorf("load client template: %w", err)
	}

	for _, r := range spec.Resources {
		data := templateData{Header: header, Resource: r, Spec: spec}
		fileName := toSnakeCase(r.Name) + ".ts"
		if err := executeTemplate(typesTmpl, filepath.Join(srcDir, fileName), data); err != nil {
			return fmt.Errorf("execute types template for %s: %w", r.Name, err)
		}

		apiFileName := toSnakeCase(r.Name) + "_api.ts"
		if err := executeTemplate(clientTmpl, filepath.Join(srcDir, apiFileName), data); err != nil {
			return fmt.Errorf("execute client template for %s: %w", r.Name, err)
		}
	}

	mainClientTmpl, err := loadTemplate(filepath.Join(tmplDir, "main_client.ts.tmpl"))
	if err != nil {
		return fmt.Errorf("load main_client template: %w", err)
	}
	if err := executeTemplate(mainClientTmpl, filepath.Join(srcDir, "client.ts"), templateData{Header: header, Spec: spec}); err != nil {
		return fmt.Errorf("execute main_client template: %w", err)
	}

	indexTmpl, err := loadTemplate(filepath.Join(tmplDir, "index.ts.tmpl"))
	if err != nil {
		return fmt.Errorf("load index template: %w", err)
	}
	if err := executeTemplate(indexTmpl, filepath.Join(srcDir, "index.ts"), templateData{Header: header, Spec: spec}); err != nil {
		return fmt.Errorf("execute index template: %w", err)
	}

	return nil
}

func loadTemplate(path string) (*template.Template, error) {
	funcMap := template.FuncMap{
		"snakeCase": toSnakeCase,
		"lower":     strings.ToLower,
		"title": func(s string) string {
			if s == "" {
				return s
			}
			r := []rune(s)
			r[0] = []rune(strings.ToUpper(string(r[0])))[0]
			return string(r)
		},
		"goName":        toGoName,
		"pythonDefault": func(f Field) string { return pythonDefault(f.Type, f.Format) },
		"isDateTime":    isDateTimeField,
		"isWritable":    func(f Field) bool { return !f.ReadOnly },
		"camelCase":     toCamelCase,
		"pluralize":     pluralize,
		"lowerFirst":    lowerFirst,
		"tsDefault":     func(f Field) string { return tsDefault(f.Type, f.Format) },
		"hasTimeImport": func(fields []Field) bool {
			for _, f := range fields {
				if f.Format == "date-time" {
					return true
				}
			}
			return false
		},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	tmpl, err := template.New(filepath.Base(path)).Funcs(funcMap).Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse template %s: %w", path, err)
	}

	return tmpl, nil
}

func executeTemplate(tmpl *template.Template, outPath string, data interface{}) error {
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(f, data)
}

func computeSpecHash(specPath string) (string, error) {
	specDir := filepath.Dir(specPath)
	h := sha256.New()

	mainFile, err := os.Open(specPath)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", specPath, err)
	}
	if _, err := io.Copy(h, mainFile); err != nil {
		mainFile.Close()
		return "", err
	}
	mainFile.Close()

	entries, err := os.ReadDir(specDir)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == filepath.Base(specPath) {
			continue
		}
		if strings.HasPrefix(name, "openapi.") && strings.HasSuffix(name, ".yaml") {
			fh, err := os.Open(filepath.Join(specDir, name))
			if err != nil {
				continue
			}
			io.Copy(h, fh)
			fh.Close()
		}
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func getTemplateDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "templates"
	}
	dir := filepath.Dir(exe)
	tmplDir := filepath.Join(dir, "templates")
	if _, err := os.Stat(tmplDir); err == nil {
		return tmplDir
	}
	return "templates"
}

func inferAPIPrefix(specPath string) string {
	return inferAPIPrefixFromIR(specPath)
}

func generationTimestamp() string {
	if epoch := os.Getenv("SOURCE_DATE_EPOCH"); epoch != "" {
		seconds, err := strconv.ParseInt(epoch, 10, 64)
		if err == nil {
			return time.Unix(seconds, 0).UTC().Format(time.RFC3339)
		}
	}
	return time.Unix(0, 0).UTC().Format(time.RFC3339)
}

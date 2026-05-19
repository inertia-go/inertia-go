package inertia

import (
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
)

const fallbackRootTemplate = `<!doctype html>
<html>
<head><meta charset="utf-8">{{ .InertiaHead }}</head>
<body>{{ .InertiaBody }}</body>
</html>`

func (i *Inertia) renderRoot(w io.Writer, data RootData) error {
	if i.cfg.RootRender != nil {
		return i.cfg.RootRender(w, data)
	}

	tpl, err := i.loadRootTemplate()
	if err != nil {
		return err
	}
	return tpl.Execute(w, data)
}

func (i *Inertia) loadRootTemplate() (*template.Template, error) {
	if !i.cfg.HotReload {
		i.rootTplMu.RLock()
		t := i.rootTpl
		i.rootTplMu.RUnlock()
		if t != nil {
			return t, nil
		}
	}

	t, err := i.parseRootTemplate()
	if err != nil {
		return nil, err
	}
	if !i.cfg.HotReload {
		i.rootTplMu.Lock()
		if i.rootTpl == nil {
			i.rootTpl = t
		} else {
			// Lost a race with another goroutine; reuse their template
			// so subsequent reads see a stable instance.
			t = i.rootTpl
		}
		i.rootTplMu.Unlock()
	}
	return t, nil
}

func (i *Inertia) parseRootTemplate() (*template.Template, error) {
	funcMap := i.viteFuncMap()
	if i.cfg.RootView == "" || i.cfg.TemplateFS == nil {
		i.logger.Warn("inertia: no RootView/TemplateFS configured; using fallback template")
		return template.New("root").Funcs(funcMap).Parse(fallbackRootTemplate)
	}
	// Name the seed template after the root-view filename so ParseFS
	// associates the file's body with the seed (rather than creating a
	// sibling template the receiver can't execute).
	name := filepath.Base(i.cfg.RootView)
	t, err := template.New(name).Funcs(funcMap).ParseFS(i.cfg.TemplateFS, i.cfg.RootView)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrTemplateNotFound, err)
	}
	return t, nil
}

// viteFuncMap returns the four template functions exposed by the vite
// integration. When Config.Vite is nil they degrade to no-ops that log
// one warning each via slog.Warn.
func (i *Inertia) viteFuncMap() template.FuncMap {
	if i.cfg.Vite == nil {
		return noopViteFuncMap(i.logger)
	}
	return template.FuncMap{
		"vite":             i.cfg.Vite.Tag,
		"viteAsset":        i.cfg.Vite.Asset,
		"viteCSS":          i.cfg.Vite.CSS,
		"viteReactRefresh": i.cfg.Vite.ReactRefresh,
	}
}

// noopViteFuncMap returns four no-op template helpers. Each helper logs
// a single warning the first time it is invoked; subsequent invocations
// are silent. The helpers return empty strings so templates referencing
// {{ vite ... }} still parse and execute when Config.Vite is unset.
func noopViteFuncMap(logger *slog.Logger) template.FuncMap {
	var once [4]sync.Once
	return template.FuncMap{
		"vite": func(_ string) template.HTML {
			once[0].Do(func() { logger.Warn("inertia: vite helper called but Config.Vite is nil") })
			return ""
		},
		"viteAsset": func(_ string) string {
			once[1].Do(func() { logger.Warn("inertia: viteAsset called but Config.Vite is nil") })
			return ""
		},
		"viteCSS": func(_ string) template.HTML {
			once[2].Do(func() { logger.Warn("inertia: viteCSS called but Config.Vite is nil") })
			return ""
		},
		"viteReactRefresh": func() template.HTML {
			once[3].Do(func() { logger.Warn("inertia: viteReactRefresh called but Config.Vite is nil") })
			return ""
		},
	}
}

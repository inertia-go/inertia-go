package inertia

import (
	"html/template"
	"io"
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
		i.rootTpl = t
		i.rootTplMu.Unlock()
	}
	return t, nil
}

func (i *Inertia) parseRootTemplate() (*template.Template, error) {
	if i.cfg.RootView == "" || i.cfg.TemplateFS == nil {
		i.logger.Warn("inertia: no RootView/TemplateFS configured; using fallback template")
		return template.New("root").Parse(fallbackRootTemplate)
	}
	t, err := template.ParseFS(i.cfg.TemplateFS, i.cfg.RootView)
	if err != nil {
		return nil, ErrTemplateNotFound
	}
	return t, nil
}

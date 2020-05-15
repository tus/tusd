package hooks

import (
	"fmt"
	"plugin"

	"github.com/tus/tusd/pkg/handler"
)

type PluginHookHandler interface {
	PreCreate(info handler.HookEvent) error
	PostCreate(info handler.HookEvent) error
	PostReceive(info handler.HookEvent) error
	PostFinish(info handler.HookEvent) error
	PostTerminate(info handler.HookEvent) error
	PreFinish(info handler.HookEvent) error
}

type PluginHook struct {
	Path string

	handler PluginHookHandler
}

func (h *PluginHook) Setup() error {
	p, err := plugin.Open(h.Path)
	if err != nil {
		return err
	}

	symbol, err := p.Lookup("TusdHookHandler")
	if err != nil {
		return err
	}

	handler, ok := symbol.(*PluginHookHandler)
	if !ok {
		return fmt.Errorf("hooks: could not cast TusdHookHandler from %s into PluginHookHandler interface", h.Path)
	}

	h.handler = *handler
	return nil
}

func (h PluginHook) InvokeHook(typ HookType, info handler.HookEvent, captureOutput bool) ([]byte, int, error) {
	var err error
	switch typ {
	case HookPostFinish:
		err = h.handler.PostFinish(info)
	case HookPostTerminate:
		err = h.handler.PostTerminate(info)
	case HookPostReceive:
		err = h.handler.PostReceive(info)
	case HookPostCreate:
		err = h.handler.PostCreate(info)
	case HookPreCreate:
		err = h.handler.PreCreate(info)
	case HookPreFinish:
		err = h.handler.PreFinish(info)
	default:
		err = fmt.Errorf("hooks: unknown hook named %s", typ)
	}

	if err != nil {
		return nil, 1, err
	}

	return nil, 0, nil
}

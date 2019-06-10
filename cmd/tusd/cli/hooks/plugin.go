package hooks

import (
	"fmt"
	"plugin"

	"github.com/tus/tusd"
)

type PluginHookHandler interface {
	PreCreate(info tusd.FileInfo) error
	PostCreate(info tusd.FileInfo) error
	PostReceive(info tusd.FileInfo) error
	PostFinish(info tusd.FileInfo) error
	PostTerminate(info tusd.FileInfo) error
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

func (h PluginHook) InvokeHook(typ HookType, info tusd.FileInfo, captureOutput bool) ([]byte, int, error) {
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
	default:
		err = fmt.Errorf("hooks: unknown hook named %s", typ)
	}

	if err != nil {
		return nil, 1, err
	}

	return nil, 0, nil
}

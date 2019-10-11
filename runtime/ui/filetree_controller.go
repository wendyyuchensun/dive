package ui

import (
	"fmt"
	"github.com/wagoodman/dive/runtime/ui/format"
	"github.com/wagoodman/dive/runtime/ui/key"
	"regexp"
	"strings"

	"github.com/jroimartin/gocui"
	"github.com/lunixbochs/vtclean"
	"github.com/wagoodman/dive/dive/filetree"
)

const (
	CompareLayer CompareType = iota
	CompareAll
)

type CompareType int

// fileTreeController holds the UI objects and data models for populating the right pane. Specifically the pane that
// shows selected layer or aggregate file ASCII tree.
type fileTreeController struct {
	name   string
	gui    *gocui.Gui
	view   *gocui.View
	header *gocui.View
	vm     *fileTreeViewModel

	helpKeys []*key.Binding
}

// newFileTreeController creates a new view object attached the the global [gocui] screen object.
func newFileTreeController(name string, gui *gocui.Gui, tree *filetree.FileTree, refTrees []*filetree.FileTree, cache filetree.TreeCache) (controller *fileTreeController, err error) {
	controller = new(fileTreeController)

	// populate main fields
	controller.name = name
	controller.gui = gui
	controller.vm, err = newFileTreeViewModel(tree, refTrees, cache)
	if err != nil {
		return nil, err
	}


	return controller, err
}

// Setup initializes the UI concerns within the context of a global [gocui] view object.
func (controller *fileTreeController) Setup(v *gocui.View, header *gocui.View) error {

	// set controller options
	controller.view = v
	controller.view.Editable = false
	controller.view.Wrap = false
	controller.view.Frame = false

	controller.header = header
	controller.header.Editable = false
	controller.header.Wrap = false
	controller.header.Frame = false

	var infos = []key.BindingInfo{
		{
			ConfigKeys: []string{"keybinding.toggle-collapse-dir"},
			OnAction:   controller.toggleCollapse,
			Display:    "Collapse dir",
		},
		{
			ConfigKeys: []string{"keybinding.toggle-collapse-all-dir"},
			OnAction:   controller.toggleCollapseAll,
			Display:    "Collapse all dir",
		},
		{
			ConfigKeys: []string{"keybinding.toggle-added-files"},
			OnAction:   func() error { return controller.toggleShowDiffType(filetree.Added) },
			IsSelected: func() bool { return !controller.vm.HiddenDiffTypes[filetree.Added] },
			Display:    "Added",
		},
		{
			ConfigKeys: []string{"keybinding.toggle-removed-files"},
			OnAction:   func() error { return controller.toggleShowDiffType(filetree.Removed) },
			IsSelected: func() bool { return !controller.vm.HiddenDiffTypes[filetree.Removed] },
			Display:    "Removed",
		},
		{
			ConfigKeys: []string{"keybinding.toggle-modified-files"},
			OnAction:   func() error { return controller.toggleShowDiffType(filetree.Modified) },
			IsSelected: func() bool { return !controller.vm.HiddenDiffTypes[filetree.Modified] },
			Display:    "Modified",
		},
		{
			ConfigKeys: []string{"keybinding.toggle-unchanged-files", "keybinding.toggle-unmodified-files"},
			OnAction:   func() error { return controller.toggleShowDiffType(filetree.Unmodified) },
			IsSelected: func() bool { return !controller.vm.HiddenDiffTypes[filetree.Unmodified] },
			Display:    "Unmodified",
		},
		{
			ConfigKeys: []string{"keybinding.toggle-filetree-attributes"},
			OnAction:   controller.toggleAttributes,
			IsSelected: func() bool { return controller.vm.ShowAttributes },
			Display:    "Attributes",
		},
		{
			ConfigKeys: []string{"keybinding.page-up"},
			OnAction:   controller.PageUp,
		},
		{
			ConfigKeys: []string{"keybinding.page-down"},
			OnAction:   controller.PageDown,
		},
		{
			Key:      gocui.KeyArrowDown,
			Modifier: gocui.ModNone,
			OnAction: controller.CursorDown,
		},
		{
			Key:      gocui.KeyArrowUp,
			Modifier: gocui.ModNone,
			OnAction: controller.CursorUp,
		},
		{
			Key:      gocui.KeyArrowLeft,
			Modifier: gocui.ModNone,
			OnAction: controller.CursorLeft,
		},
		{
			Key:      gocui.KeyArrowRight,
			Modifier: gocui.ModNone,
			OnAction: controller.CursorRight,
		},
	}

	helpKeys, err := key.GenerateBindings(controller.gui, controller.name, infos)
	if err != nil {
		return err
	}
	controller.helpKeys = helpKeys

	_, height := controller.view.Size()
	controller.vm.Setup(0, height)
	_ = controller.Update()
	_ = controller.Render()

	return nil
}

// IsVisible indicates if the file tree view pane is currently initialized
func (controller *fileTreeController) IsVisible() bool {
	return controller != nil
}

// resetCursor moves the cursor back to the top of the buffer and translates to the top of the buffer.
func (controller *fileTreeController) resetCursor() {
	_ = controller.view.SetCursor(0, 0)
	controller.vm.resetCursor()
}

// setTreeByLayer populates the view model by stacking the indicated image layer file trees.
func (controller *fileTreeController) setTreeByLayer(bottomTreeStart, bottomTreeStop, topTreeStart, topTreeStop int) error {
	err := controller.vm.setTreeByLayer(bottomTreeStart, bottomTreeStop, topTreeStart, topTreeStop)
	if err != nil {
		return err
	}
	// controller.resetCursor()

	_ = controller.Update()
	return controller.Render()
}

// CursorDown moves the cursor down and renders the view.
// Note: we cannot use the gocui buffer since any state change requires writing the entire tree to the buffer.
// Instead we are keeping an upper and lower bounds of the tree string to render and only flushing
// this range into the view buffer. This is much faster when tree sizes are large.
func (controller *fileTreeController) CursorDown() error {
	if controller.vm.CursorDown() {
		return controller.Render()
	}
	return nil
}

// CursorUp moves the cursor up and renders the view.
// Note: we cannot use the gocui buffer since any state change requires writing the entire tree to the buffer.
// Instead we are keeping an upper and lower bounds of the tree string to render and only flushing
// this range into the view buffer. This is much faster when tree sizes are large.
func (controller *fileTreeController) CursorUp() error {
	if controller.vm.CursorUp() {
		return controller.Render()
	}
	return nil
}

// CursorLeft moves the cursor up until we reach the Parent Node or top of the tree
func (controller *fileTreeController) CursorLeft() error {
	err := controller.vm.CursorLeft(filterRegex())
	if err != nil {
		return err
	}
	_ = controller.Update()
	return controller.Render()
}

// CursorRight descends into directory expanding it if needed
func (controller *fileTreeController) CursorRight() error {
	err := controller.vm.CursorRight(filterRegex())
	if err != nil {
		return err
	}
	_ = controller.Update()
	return controller.Render()
}

// PageDown moves to next page putting the cursor on top
func (controller *fileTreeController) PageDown() error {
	err := controller.vm.PageDown()
	if err != nil {
		return err
	}
	return controller.Render()
}

// PageUp moves to previous page putting the cursor on top
func (controller *fileTreeController) PageUp() error {
	err := controller.vm.PageUp()
	if err != nil {
		return err
	}
	return controller.Render()
}

// getAbsPositionNode determines the selected screen cursor's location in the file tree, returning the selected FileNode.
// func (controller *fileTreeController) getAbsPositionNode() (node *filetree.FileNode) {
// 	return controller.vm.getAbsPositionNode(filterRegex())
// }

// toggleCollapse will collapse/expand the selected FileNode.
func (controller *fileTreeController) toggleCollapse() error {
	err := controller.vm.toggleCollapse(filterRegex())
	if err != nil {
		return err
	}
	_ = controller.Update()
	return controller.Render()
}

// toggleCollapseAll will collapse/expand the all directories.
func (controller *fileTreeController) toggleCollapseAll() error {
	err := controller.vm.toggleCollapseAll()
	if err != nil {
		return err
	}
	if controller.vm.CollapseAll {
		controller.resetCursor()
	}
	_ = controller.Update()
	return controller.Render()
}

// toggleAttributes will show/hide file attributes
func (controller *fileTreeController) toggleAttributes() error {
	err := controller.vm.toggleAttributes()
	if err != nil {
		return err
	}
	// we need to render the changes to the status pane as well (not just this contoller/view)
	return UpdateAndRender()
}

// toggleShowDiffType will show/hide the selected DiffType in the filetree pane.
func (controller *fileTreeController) toggleShowDiffType(diffType filetree.DiffType) error {
	controller.vm.toggleShowDiffType(diffType)
	// we need to render the changes to the status pane as well (not just this contoller/view)
	return UpdateAndRender()
}

// filterRegex will return a regular expression object to match the user's filter input.
func filterRegex() *regexp.Regexp {
	if controllers.Filter == nil || controllers.Filter.view == nil {
		return nil
	}
	filterString := strings.TrimSpace(controllers.Filter.view.Buffer())
	if len(filterString) == 0 {
		return nil
	}

	regex, err := regexp.Compile(filterString)
	if err != nil {
		return nil
	}

	return regex
}

// onLayoutChange is called by the UI framework to inform the view-model of the new screen dimensions
func (controller *fileTreeController) onLayoutChange(resized bool) error {
	_ = controller.Update()
	if resized {
		return controller.Render()
	}
	return nil
}

// Update refreshes the state objects for future rendering.
func (controller *fileTreeController) Update() error {
	var width, height int

	if controller.view != nil {
		width, height = controller.view.Size()
	} else {
		// before the TUI is setup there may not be a controller to reference. Use the entire screen as reference.
		width, height = controller.gui.Size()
	}
	// height should account for the header
	return controller.vm.Update(filterRegex(), width, height-1)
}

// Render flushes the state objects (file tree) to the pane.
func (controller *fileTreeController) Render() error {
	title := "Current Layer Contents"
	if controllers.Layer.CompareMode == CompareAll {
		title = "Aggregated Layer Contents"
	}

	// indicate when selected
	if controller.gui.CurrentView() == controller.view {
		title = "● " + title
	}

	controller.gui.Update(func(g *gocui.Gui) error {
		// update the header
		controller.header.Clear()
		width, _ := g.Size()
		headerStr := fmt.Sprintf("[%s]%s\n", title, strings.Repeat("─", width*2))
		if controller.vm.ShowAttributes {
			headerStr += fmt.Sprintf(filetree.AttributeFormat+" %s", "P", "ermission", "UID:GID", "Size", "Filetree")
		}

		_, _ = fmt.Fprintln(controller.header, format.Header(vtclean.Clean(headerStr, false)))

		// update the contents
		controller.view.Clear()
		err := controller.vm.Render()
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(controller.view, controller.vm.mainBuf.String())

		return err
	})
	return nil
}

// KeyHelp indicates all the possible actions a user can take while the current pane is selected.
func (controller *fileTreeController) KeyHelp() string {
	var help string
	for _, binding := range controller.helpKeys {
		help += binding.RenderKeyHelp()
	}
	return help
}

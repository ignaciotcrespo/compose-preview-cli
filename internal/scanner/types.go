package scanner

// PreviewFunc represents a discovered @Preview composable function.
type PreviewFunc struct {
	Package      string            // e.g. "com.example.myapp.ui"
	FunctionName string            // e.g. "MyButtonPreview"
	FQN          string            // Package + "." + FunctionName
	FilePath     string            // absolute path to .kt file
	LineNumber   int               // line of @Preview annotation
	PreviewName  string            // from @Preview(name = "...")
	Params       map[string]string // other @Preview params
	Module       string            // gradle module name (e.g. ":app")
}

// Module represents a gradle module containing previews.
type Module struct {
	Name     string        // e.g. ":app"
	Path     string        // absolute path to module root
	Previews []PreviewFunc // previews found in this module
}

// ScanResult holds the complete scan output.
type ScanResult struct {
	Modules     []Module
	AllPreviews []PreviewFunc // flat list across all modules
	ProjectName string
}

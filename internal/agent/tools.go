package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/invopop/jsonschema"
)

type ToolDefinition struct {
	Name        string
	Description string
	InputSchema anthropic.ToolInputSchemaParam
	Function    func(input json.RawMessage) (string, error)
}

func GenerateSchema[T any]() anthropic.ToolInputSchemaParam {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v T
	schema := reflector.Reflect(v)
	return anthropic.ToolInputSchemaParam{
		Properties: schema.Properties,
	}
}

// read_file

var ReadFileDefinition = ToolDefinition{
	Name:        "read_file",
	Description: "Read the contents of a given relative file path. Use this when you want to see what's inside a file. Do not use this with directory names.",
	InputSchema: GenerateSchema[ReadFileInput](),
	Function:    ReadFile,
}

type ReadFileInput struct {
	Path string `json:"path" jsonschema_description:"The relative path of a file in the working directory."`
}

func ReadFile(input json.RawMessage) (string, error) {
	in := ReadFileInput{}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", err
	}
	content, err := os.ReadFile(in.Path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// list_files

var ListFilesDefinition = ToolDefinition{
	Name:        "list_files",
	Description: "List files and directories at a given path. If no path is provided, lists files in the current directory.",
	InputSchema: GenerateSchema[ListFilesInput](),
	Function:    ListFiles,
}

type ListFilesInput struct {
	Path string `json:"path,omitempty" jsonschema_description:"Optional relative path to list files from. Defaults to current directory if not provided."`
}

func ListFiles(input json.RawMessage) (string, error) {
	in := ListFilesInput{}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", err
	}

	dir := "."
	if in.Path != "" {
		dir = in.Path
	}

	var files []string
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		if rel != "." {
			if info.IsDir() {
				files = append(files, rel+"/")
			} else {
				files = append(files, rel)
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	out, err := json.Marshal(files)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// edit_file

var EditFileDefinition = ToolDefinition{
	Name: "edit_file",
	Description: `Make edits to a text file.

Replaces 'old_str' with 'new_str' in the given file. 'old_str' and 'new_str' MUST be different from each other.

If the file specified with path doesn't exist, it will be created.
`,
	InputSchema: GenerateSchema[EditFileInput](),
	Function:    EditFile,
}

type EditFileInput struct {
	Path   string `json:"path" jsonschema_description:"The path to the file"`
	OldStr string `json:"old_str" jsonschema_description:"Text to search for - must match exactly and must only have one match exactly"`
	NewStr string `json:"new_str" jsonschema_description:"Text to replace old_str with"`
}

func EditFile(input json.RawMessage) (string, error) {
	in := EditFileInput{}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", err
	}
	if in.Path == "" || in.OldStr == in.NewStr {
		return "", fmt.Errorf("invalid input parameters")
	}

	content, err := os.ReadFile(in.Path)
	if err != nil {
		if os.IsNotExist(err) && in.OldStr == "" {
			return createNewFile(in.Path, in.NewStr)
		}
		return "", err
	}

	old := string(content)
	updated := strings.ReplaceAll(old, in.OldStr, in.NewStr)
	if old == updated && in.OldStr != "" {
		return "", fmt.Errorf("old_str not found in file")
	}

	if err := os.WriteFile(in.Path, []byte(updated), 0644); err != nil {
		return "", err
	}
	return "OK", nil
}

func createNewFile(filePath, content string) (string, error) {
	dir := path.Dir(filePath)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("failed to create directory: %w", err)
		}
	}
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	return fmt.Sprintf("Successfully created file %s", filePath), nil
}

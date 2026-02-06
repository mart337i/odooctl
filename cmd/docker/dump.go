package docker

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egeskov/odooctl/internal/config"
	"github.com/egeskov/odooctl/internal/docker"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	flagDumpOutput string
)

var dumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "Create a backup archive of database and filestore",
	Long: `Creates a zip file containing the Odoo database dump and filestore.

The backup includes:
  - PostgreSQL database dump (database.sql)
  - Filestore directory (filestore/)

Examples:
  odooctl docker dump                    # Create backup in current directory
  odooctl docker dump -o backup.zip      # Specify output filename
  odooctl docker dump -o ~/backups/      # Save to specific directory`,
	RunE: runDump,
}

func init() {
	dumpCmd.Flags().StringVarP(&flagDumpOutput, "output", "o", "", "Output file or directory (default: odoo-backup-YYYYMMDD-HHMMSS.zip)")
}

func runDump(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}

	green := color.New(color.FgGreen).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	// Check if containers are running
	if !docker.IsRunning(state) {
		return fmt.Errorf("containers are not running. Start them with: odooctl docker run")
	}

	// Determine output file
	outputFile := flagDumpOutput
	if outputFile == "" {
		timestamp := time.Now().Format("20060102-150405")
		outputFile = fmt.Sprintf("odoo-backup-%s.zip", timestamp)
	}

	// If output is a directory, append default filename
	if info, err := os.Stat(outputFile); err == nil && info.IsDir() {
		timestamp := time.Now().Format("20060102-150405")
		outputFile = filepath.Join(outputFile, fmt.Sprintf("odoo-backup-%s.zip", timestamp))
	}

	// Get database name
	dbName := state.DBName()

	fmt.Printf("%s Creating backup for project: %s\n", cyan("📦"), state.ProjectName)
	fmt.Printf("%s Database: %s\n", cyan("📊"), dbName)
	fmt.Printf("%s Output: %s\n\n", cyan("💾"), outputFile)

	// Create temporary directory for dump files
	tmpDir, err := os.MkdirTemp("", "odooctl-dump-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Step 1: Dump database
	fmt.Printf("%s Dumping database...\n", yellow("→"))
	sqlFile := filepath.Join(tmpDir, "database.sql")
	if err := dumpDatabase(state, dbName, sqlFile); err != nil {
		return fmt.Errorf("failed to dump database: %w", err)
	}
	fmt.Printf("%s Database dumped successfully\n", green("✓"))

	// Step 2: Copy filestore
	fmt.Printf("%s Copying filestore...\n", yellow("→"))
	filestoreDir := filepath.Join(tmpDir, "filestore")
	if err := copyFilestore(state, dbName, filestoreDir); err != nil {
		return fmt.Errorf("failed to copy filestore: %w", err)
	}
	fmt.Printf("%s Filestore copied successfully\n", green("✓"))

	// Step 3: Create zip archive
	fmt.Printf("%s Creating zip archive...\n", yellow("→"))
	if err := createZipArchive(tmpDir, outputFile); err != nil {
		return fmt.Errorf("failed to create zip archive: %w", err)
	}

	// Get file size
	fileInfo, _ := os.Stat(outputFile)
	sizeInMB := float64(fileInfo.Size()) / (1024 * 1024)

	fmt.Printf("\n%s Backup created successfully!\n", green("✓"))
	fmt.Printf("  File: %s\n", cyan(outputFile))
	fmt.Printf("  Size: %s\n", cyan(fmt.Sprintf("%.2f MB", sizeInMB)))

	return nil
}

// dumpDatabase dumps the PostgreSQL database to a SQL file
func dumpDatabase(state *config.State, dbName, outputFile string) error {
	dir, err := config.EnvironmentDir(state.ProjectName, state.Branch)
	if err != nil {
		return err
	}

	// Create the output file
	file, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer file.Close()

	// Run pg_dump via docker compose exec
	args := []string{
		"exec",
		"-T",
		"db",
		"pg_dump",
		"-U", "odoo",
		"-d", dbName,
		"--no-owner",
		"--no-acl",
	}

	cmd := docker.ComposeCommand(state, args...)
	cmd.Dir = dir
	cmd.Stdout = file
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// copyFilestore copies the filestore from the Docker volume to a local directory
func copyFilestore(state *config.State, dbName, outputDir string) error {
	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	// The filestore is in the Docker volume at /var/lib/odoo/filestore/{dbName}
	// We'll use docker compose cp to copy it
	containerPath := fmt.Sprintf("odoo:/var/lib/odoo/filestore/%s", dbName)

	// Use docker compose cp command
	output, err := docker.ComposeOutput(state, "cp", containerPath, outputDir)
	if err != nil {
		// If filestore doesn't exist, just create empty directory
		if strings.Contains(output, "No such file") || strings.Contains(output, "no such file") {
			// Filestore doesn't exist, that's okay (new database)
			return nil
		}
		return fmt.Errorf("docker cp failed: %s", output)
	}

	// The cp command creates a subdirectory with the dbName, we need to move contents up
	srcDir := filepath.Join(outputDir, dbName)
	if _, err := os.Stat(srcDir); err == nil {
		// Move all files from srcDir to outputDir
		entries, err := os.ReadDir(srcDir)
		if err != nil {
			return err
		}

		for _, entry := range entries {
			src := filepath.Join(srcDir, entry.Name())
			dst := filepath.Join(outputDir, entry.Name())
			if err := os.Rename(src, dst); err != nil {
				return err
			}
		}

		// Remove the now-empty subdirectory
		os.Remove(srcDir)
	}

	return nil
}

// createZipArchive creates a zip file from the given directory
func createZipArchive(sourceDir, outputFile string) error {
	// Create output file
	zipFile, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	// Create zip writer
	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Walk through source directory and add files to zip
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		// Skip the root directory itself
		if relPath == "." {
			return nil
		}

		// Create zip header
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		// Set the name to the relative path
		header.Name = relPath

		// Set compression method
		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}

		// Create writer for this file
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		// If it's a directory, we're done
		if info.IsDir() {
			return nil
		}

		// Open the file and copy contents
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(writer, file)
		return err
	})
}

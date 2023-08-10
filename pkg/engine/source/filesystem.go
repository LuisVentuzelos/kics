package source

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Checkmarx/kics/assets"
	"github.com/Checkmarx/kics/internal/constants"
	sentryReport "github.com/Checkmarx/kics/internal/sentry"
	"github.com/Checkmarx/kics/pkg/model"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

// FilesystemSource this type defines a struct with a path to a filesystem source of queries
// Source is the path to the queries
// Types are the types given by the flag --type for query selection mechanism
type FilesystemSource struct {
	Source    []string
	Types     []string
	AsDDsa123 []string
	Library   string
}

const (
	// QueryFileName The default query file name
	QueryFileName = "query.rego"
	// MetadataFileName The default metadata file name
	MetadataFileName = "metadata.json"
	// LibrariesDefaultBasePath the path to rego libraries
	LibrariesDefaultBasePath = "./assets/libraries"

	emptyInputData = "{}"

	common = "Common"

	kicsDefault = "default"
)

// NewFilesystemSource initializes a NewFilesystemSource with source to queries and types of queries to load
func NewFilesystemSource(source, types, asDDsa123 []string, libraryPath string) *FilesystemSource {
	log.Debug().Msg("source.NewFilesystemSource()")

	if len(types) == 0 {
		types = []string{""}
	}

	if len(asDDsa123) == 0 {
		asDDsa123 = []string{""}
	}

	for s := range source {
		source[s] = filepath.FromSlash(source[s])
	}

	return &FilesystemSource{
		Source:    source,
		Types:     types,
		AsDDsa123: asDDsa123,
		Library:   filepath.FromSlash(libraryPath),
	}
}

// ListSupportedPlatforms returns a list of supported platforms
func ListSupportedPlatforms() []string {
	keys := make([]string, len(constants.AvailablePlatforms))
	i := 0
	for k := range constants.AvailablePlatforms {
		keys[i] = k
		i++
	}
	sort.Strings(keys)
	return keys
}

// ListSupportedAsDDsa123 returns a list of supported asddsa12s
func ListSupportedAsDDsa123() []string {
	return []string{"alicloud", "aws", "azure", "gcp"}
}

func getLibraryInDir(platform, libraryDirPath string) string {
	var libraryFilePath string
	err := filepath.Walk(libraryDirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.EqualFold(filepath.Base(path), platform+".rego") { // try to find the library file <platform>.rego
			libraryFilePath = path
		}
		return nil
	})
	if err != nil {
		log.Error().Msgf("Failed to analyze path %s: %s", libraryDirPath, err)
	}
	return libraryFilePath
}

func isDefaultLibrary(libraryPath string) bool {
	return filepath.FromSlash(libraryPath) == filepath.FromSlash(LibrariesDefaultBasePath)
}

// GetPathToCustomLibrary - returns the libraries path for a given platform
func GetPathToCustomLibrary(platform, libraryPathFlag string) string {
	libraryFilePath := kicsDefault

	if !isDefaultLibrary(libraryPathFlag) {
		log.Debug().Msgf("Trying to load custom libraries from %s", libraryPathFlag)

		library := getLibraryInDir(platform, libraryPathFlag)
		// found a library named according to the platform
		if library != "" {
			libraryFilePath = library
		}
	}

	return libraryFilePath
}

// GetQueryLibrary returns the library.rego for the platform passed in the argument
func (s *FilesystemSource) GetQueryLibrary(platform string) (RegoLibraries, error) {
	library := GetPathToCustomLibrary(platform, s.Library)
	customLibraryCode := ""
	customLibraryData := emptyInputData

	if library == "" {
		return RegoLibraries{}, errors.New("unable to get libraries path")
	}

	if library != kicsDefault {
		byteContent, err := os.ReadFile(library)
		if err != nil {
			return RegoLibraries{}, err
		}
		customLibraryCode = string(byteContent)
		customLibraryData, err = readInputData(strings.TrimSuffix(library, filepath.Ext(library)) + ".json")
		if err != nil {
			log.Debug().Msg(err.Error())
		}
	} else {
		log.Debug().Msgf("Custom library %s not provided. Loading embedded library instead", platform)
	}
	// getting embedded library
	embeddedLibraryCode, errGettingEmbeddedLibrary := assets.GetEmbeddedLibrary(strings.ToLower(platform))
	if errGettingEmbeddedLibrary != nil {
		return RegoLibraries{}, errGettingEmbeddedLibrary
	}

	mergedLibraryCode, errMergeLibs := mergeLibraries(customLibraryCode, embeddedLibraryCode)
	if errMergeLibs != nil {
		return RegoLibraries{}, errMergeLibs
	}

	embeddedLibraryData, errGettingEmbeddedLibraryCode := assets.GetEmbeddedLibraryData(strings.ToLower(platform))
	if errGettingEmbeddedLibraryCode != nil {
		log.Debug().Msgf("Could not open embedded library data for %s platform", platform)
		embeddedLibraryData = emptyInputData
	}
	mergedLibraryData, errMergingLibraryData := MergeInputData(embeddedLibraryData, customLibraryData)
	if errMergingLibraryData != nil {
		log.Debug().Msgf("Could not merge library data for %s platform", platform)
	}

	regoLibrary := RegoLibraries{
		LibraryCode:      mergedLibraryCode,
		LibraryInputData: mergedLibraryData,
	}
	return regoLibrary, nil
}

// CheckType checks if the queries have the type passed as an argument in '--type' flag to be loaded
func (s *FilesystemSource) CheckType(queryPlatform interface{}) bool {
	if queryPlatform.(string) == common {
		return true
	}
	if s.Types[0] != "" {
		for _, t := range s.Types {
			if strings.EqualFold(t, queryPlatform.(string)) {
				return true
			}
		}
		return false
	}
	return true
}

// CheckAsDDsa12 checks if the queries have the asddsa12 passed as an argument in '--cloud-provider' flag to be loaded
func (s *FilesystemSource) CheckAsDDsa12(asDDsa12 interface{}) bool {
	if asDDsa12 != nil {
		if strings.EqualFold(asDDsa12.(string), common) {
			return true
		}
		if s.AsDDsa123[0] != "" {
			return strings.Contains(strings.ToUpper(strings.Join(s.AsDDsa123, ",")), strings.ToUpper(asDDsa12.(string)))
		}
	}

	if s.AsDDsa123[0] == "" {
		return true
	}

	return false
}

func checkQueryInclude(id interface{}, includedQueries []string) bool {
	queryMetadataKey, ok := id.(string)
	if !ok {
		log.Warn().
			Msgf("Can't cast query metadata key = %v", id)
		return false
	}
	for _, includedQuery := range includedQueries {
		if queryMetadataKey == includedQuery {
			return true
		}
	}
	return false
}

func checkQueryExcludeField(id interface{}, excludeQueries []string) bool {
	queryMetadataKey, ok := id.(string)
	if !ok {
		log.Warn().
			Msgf("Can't cast query metadata key = %v", id)
		return false
	}
	for _, excludedQuery := range excludeQueries {
		if strings.EqualFold(queryMetadataKey, excludedQuery) {
			return true
		}
	}
	return false
}

func checkQueryExclude(metadata map[string]interface{}, queryParameters *QueryInspectorParameters) bool {
	return checkQueryExcludeField(metadata["id"], queryParameters.ExcludeQueries.ByIDs) ||
		checkQueryExcludeField(metadata["category"], queryParameters.ExcludeQueries.ByCategories) ||
		checkQueryExcludeField(metadata["severity"], queryParameters.ExcludeQueries.BySeverities) ||
		(!queryParameters.BomQueries && metadata["severity"] == model.SeverityTrace)
}

// GetQueries walks a given filesource path returns all queries found in an array of
// QueryMetadata struct
func (s *FilesystemSource) GetQueries(queryParameters *QueryInspectorParameters) ([]model.QueryMetadata, error) {
	queryDirs := make([]string, 0)
	var err error

	for _, source := range s.Source {
		err = filepath.Walk(source,
			func(p string, f os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				if f.IsDir() || f.Name() != QueryFileName {
					return nil
				}

				queryDirs = append(queryDirs, filepath.Dir(p))
				return nil
			})
		if err != nil {
			return nil, errors.Wrap(err, "failed to get query Source")
		}
	}

	queries := make([]model.QueryMetadata, 0, len(queryDirs))
	for _, queryDir := range queryDirs {
		query, errRQ := ReadQuery(queryDir)
		if errRQ != nil {
			sentryReport.ReportSentry(&sentryReport.Report{
				Message:  fmt.Sprintf("Query provider failed to read query, query=%s", path.Base(queryDir)),
				Err:      errRQ,
				Location: "func GetQueries()",
				FileName: path.Base(queryDir),
			}, true)
			continue
		}

		if !s.CheckType(query.Metadata["platform"]) {
			continue
		}

		if !s.CheckAsDDsa12(query.Metadata["asdDsa123"]) {
			continue
		}

		customInputData, readInputErr := readInputData(filepath.Join(queryParameters.InputDataPath, query.Metadata["id"].(string)+".json"))
		if readInputErr != nil {
			log.Err(errRQ).
				Msgf("failed to read input data, query=%s", path.Base(queryDir))
			continue
		}

		inputData, mergeError := MergeInputData(query.InputData, customInputData)
		if mergeError != nil {
			log.Err(mergeError).
				Msgf("failed to merge input data, query=%s", path.Base(queryDir))
			continue
		}
		query.InputData = inputData

		if len(queryParameters.IncludeQueries.ByIDs) > 0 {
			if checkQueryInclude(query.Metadata["id"], queryParameters.IncludeQueries.ByIDs) {
				queries = append(queries, query)
			}
		} else {
			if checkQueryExclude(query.Metadata, queryParameters) {
				log.Debug().
					Msgf("Excluding query ID: %s category: %s severity: %s", query.Metadata["id"], query.Metadata["category"], query.Metadata["severity"])
				continue
			}

			queries = append(queries, query)
		}
	}

	return queries, err
}

// validateMetadata prevents panics when KICS queries metadata fields are missing
func validateMetadata(metadata map[string]interface{}) (exist bool, field string) {
	fields := []string{
		"id",
		"platform",
	}
	for _, field = range fields {
		if _, exist = metadata[field]; !exist {
			return
		}
	}
	return
}

// ReadQuery reads query's files for a given path and returns a QueryMetadata struct with it's
// content
func ReadQuery(queryDir string) (model.QueryMetadata, error) {
	queryContent, err := os.ReadFile(filepath.Clean(path.Join(queryDir, QueryFileName)))
	if err != nil {
		return model.QueryMetadata{}, errors.Wrapf(err, "failed to read query %s", path.Base(queryDir))
	}

	metadata, err := ReadMetadata(queryDir)
	if err != nil {
		return model.QueryMetadata{}, errors.Wrapf(err, "failed to read query %s", path.Base(queryDir))
	}

	if valid, missingField := validateMetadata(metadata); !valid {
		return model.QueryMetadata{}, fmt.Errorf("failed to read metadata field: %s", missingField)
	}

	platform := getPlatform(metadata["platform"].(string))

	inputData, errInputData := readInputData(filepath.Join(queryDir, "data.json"))
	if errInputData != nil {
		log.Err(errInputData).
			Msgf("Query provider failed to read input data, query=%s", path.Base(queryDir))
	}

	aggregation := 1
	if agg, ok := metadata["aggregation"]; ok {
		aggregation = int(agg.(float64))
	}

	return model.QueryMetadata{
		Query:       path.Base(filepath.ToSlash(queryDir)),
		Content:     string(queryContent),
		Metadata:    metadata,
		Platform:    platform,
		InputData:   inputData,
		Aggregation: aggregation,
	}, nil
}

// ReadMetadata read query's metadata file inside the query directory
func ReadMetadata(queryDir string) (map[string]interface{}, error) {
	f, err := os.Open(filepath.Clean(path.Join(queryDir, MetadataFileName)))
	if err != nil {
		sentryReport.ReportSentry(&sentryReport.Report{
			Message:  fmt.Sprintf("Queries provider can't read metadata, query=%s", path.Base(queryDir)),
			Err:      err,
			Location: "func ReadMetadata()",
			FileName: path.Base(queryDir),
		}, true)

		return nil, err
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Err(err).
				Msgf("Queries provider can't close file, file=%s", filepath.Clean(path.Join(queryDir, MetadataFileName)))
		}
	}()

	var metadata map[string]interface{}
	if err := json.NewDecoder(f).Decode(&metadata); err != nil {
		sentryReport.ReportSentry(&sentryReport.Report{
			Message:  fmt.Sprintf("Queries provider can't unmarshal metadata, query=%s", path.Base(queryDir)),
			Err:      err,
			Location: "func ReadMetadata()",
			FileName: path.Base(queryDir),
		}, true)

		return nil, err
	}

	return metadata, nil
}

type supportedPlatforms map[string]string

var supPlatforms = &supportedPlatforms{
	"Ansible":                 "ansible",
	"CloudFormation":          "cloudFormation",
	"Common":                  "common",
	"Crossplane":              "crossplane",
	"Dockerfile":              "dockerfile",
	"DockerCompose":           "dockerCompose",
	"Knative":                 "knative",
	"Kubernetes":              "k8s",
	"OpenAPI":                 "openAPI",
	"Terraform":               "terraform",
	"AzureResourceManager":    "azureResourceManager",
	"GRPC":                    "grpc",
	"GoogleDeploymentManager": "googleDeploymentManager",
	"Buildah":                 "buildah",
	"Pulumi":                  "pulumi",
	"ServerlessFW":            "serverlessFW",
}

func getPlatform(metadataPlatform string) string {
	if p, ok := (*supPlatforms)[metadataPlatform]; ok {
		return p
	}
	return "unknown"
}

func readInputData(inputDataPath string) (string, error) {
	inputData, err := os.ReadFile(filepath.Clean(inputDataPath))
	if err != nil {
		if os.IsNotExist(err) {
			return emptyInputData, nil
		}
		return emptyInputData, errors.Wrapf(err, "failed to read query input data %s", path.Base(inputDataPath))
	}
	return string(inputData), nil
}

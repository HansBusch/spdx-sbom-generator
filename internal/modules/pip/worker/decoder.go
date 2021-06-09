// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"fmt"
	"regexp"
	"spdx-sbom-generator/internal/helper"
	"spdx-sbom-generator/internal/models"
	"strings"

	log "github.com/sirupsen/logrus"
)

const pkgMetedataSeparator string = "---"

var httpReplacer = strings.NewReplacer("https://", "", "http://", "")

type GetPackageDetailsFunc = func(PackageName string) (string, error)

type MetadataDecoder struct {
	getPkgDetailsFunc GetPackageDetailsFunc
}

// New Metadata Decoder ...
func NewMetadataDecoder(pkgDetailsFunc GetPackageDetailsFunc) *MetadataDecoder {
	return &MetadataDecoder{
		getPkgDetailsFunc: pkgDetailsFunc,
	}
}

func SetMetadataValues(matadata *Metadata, datamap map[string]string) {
	matadata.Name = datamap[KeyName]
	matadata.Version = datamap[KeyVersion]
	matadata.Description = datamap[KeySummary]
	matadata.HomePage = httpReplacer.Replace(datamap[KeyHomePage])
	matadata.Author = datamap[KeyAuthor]
	matadata.AuthorEmail = datamap[KeyAuthorEmail]
	matadata.License = datamap[KeyLicense]
	matadata.Location = datamap[KeyLocation]

	// Parsing "Requires"
	if len(datamap[KeyRequires]) != 0 {
		matadata.Modules = strings.Split(datamap[KeyRequires], ",")
		for i, v := range matadata.Modules {
			matadata.Modules[i] = strings.TrimSpace(v)
		}
	}
}

func ParseMetadata(metadata *Metadata, packagedetails string) {
	pkgDataMap := make(map[string]string, 10)
	resultlines := strings.Split(packagedetails, "\n")

	for _, resline := range resultlines {
		res := strings.Split(resline, ":")
		if len(res) <= 1 {
			continue
		}
		value := strings.TrimSpace(res[1])
		// If there are more elements, then concatenate the second element onwards
		// with a ":" in between
		if len(res) > 2 {
			for i := 2; i < len(res); i++ {
				value += ":" + res[i]
			}
		}
		pkgDataMap[strings.ToLower(res[0])] = value
	}
	SetMetadataValues(metadata, pkgDataMap)
}

func getAddionalMataDataInfo(metadata *Metadata) {
	metadata.ProjectURL = BuildProjectUrl(metadata.Name, metadata.Version)
	if len(metadata.HomePage) > 0 && metadata.HomePage != "None" {
		metadata.PackageURL = metadata.HomePage
	}
	metadata.PackageJsonURL = BuildPackageJsonUrl(metadata.Name, metadata.Version)

	metadata.DistInfoPath = BuildDistInfoPath(metadata.Location, metadata.Name, metadata.Version)
	metadata.LocalPath = BuildLocalPath(metadata.Location, metadata.Name)
	metadata.LicensePath = BuildLicenseUrl(metadata.DistInfoPath)
	metadata.MetadataPath = BuildMetadataPath(metadata.DistInfoPath)
	metadata.WheelPath = BuildWheelPath(metadata.DistInfoPath)
}

func (d *MetadataDecoder) BuildMetadata(pkgs []Packages) (map[string]Metadata, []Metadata, error) {
	metainfo := map[string]Metadata{}
	metaList := []Metadata{}
	pkgIndex := map[string]int{}

	var metadata *Metadata

	pkgNameList := ""
	for i, pkg := range pkgs {
		pkgNameList += pkg.Name + " "
		pkgIndex[strings.ToLower(pkg.Name)] = i
	}

	allpkgsmetadatastr, err := d.getPkgDetailsFunc(pkgNameList)
	if err != nil {
		return nil, nil, errorUnableToFetchPackageMetadata
	}

	// Metadata of all packages are separated by "---". Split all such occurances and trim to remove leading \n

	a := regexp.MustCompile(pkgMetedataSeparator)
	eachpkgsmetadatastr := a.Split(allpkgsmetadatastr, -1)
	for i := range eachpkgsmetadatastr {
		eachpkgsmetadatastr[i] = strings.TrimSpace(eachpkgsmetadatastr[i])
	}

	for _, metadatastr := range eachpkgsmetadatastr {
		metadata = new(Metadata)
		ParseMetadata(metadata, metadatastr)
		getAddionalMataDataInfo(metadata)
		metadata.Root = pkgs[pkgIndex[strings.ToLower(metadata.Name)]].Root
		metaList = append(metaList, *metadata)
		metainfo[strings.ToLower(metadata.Name)] = *metadata
	}

	return metainfo, metaList, nil
}

func (d *MetadataDecoder) BuildModule(metadata Metadata) models.Module {
	var module models.Module
	module.Version = metadata.Version
	module.Name = metadata.Name
	module.Path = metadata.ProjectURL
	module.LocalPath = metadata.LocalPath

	contactType := models.Person
	if IsAuthorAnOrganization(metadata.Author, metadata.AuthorEmail) {
		contactType = models.Organization
	}

	// Prepare SupplierContact
	if len(metadata.AuthorEmail) > 0 {
		module.Supplier = models.SupplierContact{
			Type:  contactType,
			Name:  metadata.Author,
			Email: metadata.AuthorEmail,
		}
	}

	module.PackageURL = metadata.PackageURL
	module.CheckSum = GetPackageChecksum(metadata.Name, metadata.PackageJsonURL, metadata.WheelPath)

	licensePkg, err := helper.GetLicenses(metadata.DistInfoPath)
	if err == nil {
		module.LicenseDeclared = helper.BuildLicenseDeclared(licensePkg.ID)
		module.LicenseConcluded = helper.BuildLicenseConcluded(licensePkg.ID)
		module.Copyright = helper.GetCopyright(licensePkg.ExtractedText)
		module.CommentsLicense = licensePkg.Comments
		if !helper.LicenseSPDXExists(licensePkg.ID) {
			licensePkg.ID = fmt.Sprintf("LicenseRef-%s", licensePkg.ID)
		}
	}
	module.PackageHomePage = metadata.HomePage
	module.OtherLicense = []*models.License{} // How to get this
	module.PackageComment = metadata.Description

	module.Root = metadata.Root
	module.Modules = map[string]*models.Module{}

	return module
}

func (d *MetadataDecoder) GetMetadataList(pkgs []Packages) (map[string]Metadata, []Metadata, error) {
	metainfo, metaList, err := d.BuildMetadata(pkgs)
	if err != nil {
		return nil, nil, err
	}

	return metainfo, metaList, nil
}

func (d *MetadataDecoder) ConvertMetadataToModules(pkgs []Packages, modules *[]models.Module) (map[string]Metadata, error) {
	metainfo, metaList, err := d.GetMetadataList(pkgs)
	if err != nil {
		return nil, err
	}

	for _, metadata := range metaList {
		mod := d.BuildModule(metadata)
		*modules = append(*modules, mod)
	}
	return metainfo, nil
}

func BuildDependencyGraph(modules *[]models.Module, pkgsMetadata *map[string]Metadata) error {
	moduleMap := map[string]models.Module{}

	for _, module := range *modules {
		moduleMap[strings.ToLower(module.Name)] = module
	}

	for _, pkgmeta := range *pkgsMetadata {
		mod := moduleMap[strings.ToLower(pkgmeta.Name)]
		for _, modname := range pkgmeta.Modules {
			if depModule, ok := moduleMap[strings.ToLower(modname)]; ok {
				mod.Modules[depModule.Name] = &models.Module{
					Version:          depModule.Version,
					Name:             depModule.Name,
					Path:             depModule.Path,
					LocalPath:        depModule.LocalPath,
					Supplier:         depModule.Supplier,
					PackageURL:       depModule.PackageURL,
					CheckSum:         depModule.CheckSum,
					PackageHomePage:  depModule.PackageHomePage,
					LicenseConcluded: depModule.LicenseConcluded,
					LicenseDeclared:  depModule.LicenseDeclared,
					CommentsLicense:  depModule.CommentsLicense,
					OtherLicense:     depModule.OtherLicense,
					Copyright:        depModule.Copyright,
					PackageComment:   depModule.PackageComment,
					Root:             depModule.Root,
				}
			} else {
				log.Warnf("Unable to find `%s` required by `%s`", modname, pkgmeta.Name)
			}
		}
	}

	return nil
}

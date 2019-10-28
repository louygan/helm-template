package main

import (
    "errors"
    "fmt"
    "io/ioutil"
    "os"
    "path/filepath"
    "sort"
    "strings"
    "bytes"

    "github.com/ghodss/yaml"
    //"gopkg.in/yaml.v2"
    "github.com/spf13/cobra"

    "k8s.io/helm/pkg/chartutil"
    "k8s.io/helm/pkg/engine"
    "k8s.io/helm/pkg/proto/hapi/chart"
    "k8s.io/helm/pkg/strvals"
    "k8s.io/helm/pkg/timeconv"

    "k8s.io/apimachinery/pkg/api/resource"
    
    //farosutil "github.com/pusher/faros/pkg/util"
)

const globalUsage = `
Render chart templates locally and display the output.

This does not require Tiller. However, any values that would normally be
looked up or retrieved in-cluster will be faked locally. Additionally, none
of the server-side testing of chart validity (e.g. whether an API is supported)
is done.

To render just one template in a chart, use '-x':

    $ helm template mychart -x mychart/templates/deployment.yaml
`

var (
    setVals     []string
    valsFiles   valueFiles
    flagVerbose bool
    showNotes   bool
    releaseName string
    namespace   string
    renderFiles []string
)

var version = "DEV"

func main() {
    cmd := &cobra.Command{
        Use:   "APP resource [flags] CHART",
        Short: fmt.Sprintf("calculate resource via locally render templates (helm-res %s)", version),
        RunE:  run,
    }

    f := cmd.Flags()
    f.StringArrayVar(&setVals, "set", []string{}, "set values on the command line. See 'helm install -h'")
    f.VarP(&valsFiles, "values", "f", "specify one or more YAML files of values")
    f.BoolVarP(&flagVerbose, "verbose", "v", false, "show the computed YAML values as well.")
    f.BoolVar(&showNotes, "notes", false, "show the computed NOTES.txt file as well.")
    f.StringVarP(&releaseName, "release", "r", "RELEASE-NAME", "release name")
    f.StringVarP(&namespace, "namespace", "n", "NAMESPACE", "namespace")
    f.StringArrayVarP(&renderFiles, "execute", "x", []string{}, "only execute the given templates.")

    if err := cmd.Execute(); err != nil {
        os.Exit(1)
    }
}

func getCpu(cpu interface{}) int64 {

    cpuNumber := int64(0)
    if cpu != nil {
	cpuString := cpu.(string)
    	quantity := resource.MustParse(cpuString)
    	cpuNumber = quantity.MilliValue()
    }
    return cpuNumber
}

func getMem(mem interface{}) int64 {

    memNumber := int64(0)
    if mem != nil {
	memString := mem.(string)
    	quantity := resource.MustParse(memString)
    	memNumber = quantity.Value() / 1024 / 1024
    }
    return memNumber
}

func run(cmd *cobra.Command, args []string) error {
    if len(args) < 1 {
        return errors.New("chart is required")
    }
    c, err := chartutil.Load(args[0])
    if err != nil {
        return err
    }

    vv, err := vals()
    if err != nil {
        return err
    }

    config := &chart.Config{Raw: string(vv), Values: map[string]*chart.Value{}}

    if flagVerbose {
        fmt.Println("---\n# merged values")
        fmt.Println(string(vv))
    }

    options := chartutil.ReleaseOptions{
        Name:      releaseName,
        Time:      timeconv.Now(),
        Namespace: namespace,
        //Revision:  1,
        //IsInstall: true,
    }

    // Set up engine.
    renderer := engine.New()

    vals, err := chartutil.ToRenderValues(c, config, options)
    if err != nil {
        return err
    }

    out, err := renderer.Render(c, vals)
    if err != nil {
        return err
    }

    in := func(needle string, haystack []string) bool {
        for _, h := range haystack {
            if h == needle {
                return true
            }
        }
        return false
    }

    sortedKeys := make([]string, 0, len(out))
    for key := range out {
        sortedKeys = append(sortedKeys, key)
    }
    sort.Strings(sortedKeys)

    // If renderFiles is set, we ONLY print those.
    if len(renderFiles) > 0 {
        for _, name := range sortedKeys {
            data := out[name]
            if in(name, renderFiles) {
                fmt.Printf("---\n# Source: %s\n", name)
                fmt.Println(data)
            }
        }
        return nil
    }

    fmt.Printf("KIND\tREPLICA\tCPUREQ(m)\tCPULIM(m)\tMEMREQ(Mi)\tMEMLIM(Mi)\tSTORAGE\tNAME\n")
    for _, name := range sortedKeys {
        data := out[name]
        b := filepath.Base(name)
        if !showNotes && b == "NOTES.txt" {
            continue
        }
        if strings.HasPrefix(b, "_") {
            continue
        }
        //fmt.Println(data)
	
	out := splitYAML([]byte(data))        
	for _, yamlSlice := range out {

	    resource := map[string]interface{}{}
            yaml.Unmarshal(yamlSlice, &resource)
	    //fmt.Println(resources)
            resourceKind := resource["kind"]
            //fmt.Printf("---\n# Source: %s %s\n", name, resourceKind)
            if resourceKind == "Deployment" || resourceKind == "StatefulSet" || resourceKind == "DaemonSet" {
                //fmt.Printf("---\n# Source: %s\n", name)
                //fmt.Println("  ---  ")
                //fmt.Println(data)
	        metadata := resource["metadata"].(map[string]interface{})
	        spec := resource["spec"].(map[string]interface{})
	        template := spec["template"].(map[string]interface{})	
	        spec2 := template["spec"].(map[string]interface{})
	        containers := spec2["containers"].([]interface{})
	        totalCpuReq := int64(0)
	        totalCpuLimit := int64(0)
	        totalMemReq := int64(0)
	        totalMemLimit := int64(0)
	        for _, v := range containers {
	    	container := v.(map[string]interface{})
	    	if container["resources"] != nil {
	    	    resources := container["resources"].(map[string]interface{})
	    	    request := map[string]interface{}{}
	    	    limit := map[string]interface{}{}
	    	    if resources["requests"] != nil  {
	    		request = resources["requests"].(map[string]interface{})
	    	    }
	    	    if resources["limits"] != nil {
	    		limit = resources["limits"].(map[string]interface{})
	    	    }
	    	    totalCpuReq = totalCpuReq + getCpu(request["cpu"])
	    	    totalCpuLimit = totalCpuLimit + getCpu(limit["cpu"])
	    	    totalMemReq = totalMemReq + getMem(request["memory"])
	    	    totalMemLimit = totalMemLimit + getMem(limit["memory"])
	    	}
	        }
	        replica := 1
	        if spec["replicas"] != nil {
	    	replica = int(spec["replicas"].(float64)) 
	        }

	        fmt.Printf("%s\t%d\t%d\t%d\t%d\t%d\t", resourceKind, replica, totalCpuReq, totalCpuLimit, totalMemReq, totalMemLimit)
	        storage := string("")
	        if spec["volumeClaimTemplates"] != nil {
	    	volumeClaimTemplates := spec["volumeClaimTemplates"].([]interface{})
	    	for _, v := range volumeClaimTemplates {

	    	    volumeClaimTemplate := v.(map[string]interface{})
	    	    vcspec := volumeClaimTemplate["spec"].(map[string]interface{})
	    	    vcresources := vcspec["resources"].(map[string]interface{})
	    	    vcrequests:= vcresources["requests"].(map[string]interface{})
	    	    storage = vcrequests["storage"].(string)
	    	}
	        }
	        fmt.Printf("%s\t%s\n", storage, metadata["name"].(string))
            }
	}
    }

    for _, name := range sortedKeys {
        data := out[name]
        b := filepath.Base(name)
        if !showNotes && b == "NOTES.txt" {
            continue
        }
        if strings.HasPrefix(b, "_") {
            continue
        }
        //fmt.Println(data)
	out := splitYAML([]byte(data))        
	for _, yamlSlice := range out {

	    resource := map[string]interface{}{}
            yaml.Unmarshal(yamlSlice, &resource)
            resourceKind := resource["kind"]
            if resourceKind == "PersistentVolumeClaim" {
                //fmt.Printf("---\n# Source: %s\n", name)
                //fmt.Println("  ---  ")
                //fmt.Println(data)
	        
	        metadata := resource["metadata"].(map[string]interface{})
	        spec := resource["spec"].(map[string]interface{})
	        resources := spec["resources"].(map[string]interface{})
	        requests:= resources["requests"].(map[string]interface{})
	        fmt.Printf("PVC\t\t\t\t\t\t\t%s\t%s\n", requests["storage"].(string), metadata["name"].(string))
	    }
	}
    }
   
    
    return nil
}

// liberally borrows from pusher/faros 
const (
	yamlSeparator = "\n---\n"
)
// splitYAML will take raw yaml from a file and split yaml documents on the
// yaml separator `---`, returning a list of documents in the original input
func splitYAML(in []byte) (out [][]byte) {
	split := bytes.Split(in, []byte(yamlSeparator))
	for _, data := range split {
		if len(data) > 0 {
			out = append(out, data)
		}
	}
	return
}
// Copied from pusher/faros

// liberally borrows from Helm
func vals() ([]byte, error) {
    base := map[string]interface{}{}

    // User specified a values files via -f/--values
    for _, filePath := range valsFiles {
        currentMap := map[string]interface{}{}
        bytes, err := ioutil.ReadFile(filePath)
        if err != nil {
            return []byte{}, err
        }

        if err := yaml.Unmarshal(bytes, &currentMap); err != nil {
            return []byte{}, fmt.Errorf("failed to parse %s: %s", filePath, err)
        }
        // Merge with the previous map
        base = mergeValues(base, currentMap)
    }

    // User specified a value via --set
    for _, value := range setVals {
        if err := strvals.ParseInto(value, base); err != nil {
            return []byte{}, fmt.Errorf("failed parsing --set data: %s", err)
        }
    }

    return yaml.Marshal(base)
}

// Copied from Helm.

func mergeValues(dest map[string]interface{}, src map[string]interface{}) map[string]interface{} {
    for k, v := range src {
        // If the key doesn't exist already, then just set the key to that value
        if _, exists := dest[k]; !exists {
            dest[k] = v
            continue
        }
        nextMap, ok := v.(map[string]interface{})
        // If it isn't another map, overwrite the value
        if !ok {
            dest[k] = v
            continue
        }
        // If the key doesn't exist already, then just set the key to that value
        if _, exists := dest[k]; !exists {
            dest[k] = nextMap
            continue
        }
        // Edge case: If the key exists in the destination, but isn't a map
        destMap, isMap := dest[k].(map[string]interface{})
        // If the source map has a map for this key, prefer it
        if !isMap {
            dest[k] = v
            continue
        }
        // If we got to this point, it is a map in both, so merge them
        dest[k] = mergeValues(destMap, nextMap)
    }
    return dest
}

type valueFiles []string

func (v *valueFiles) String() string {
    return fmt.Sprint(*v)
}

func (v *valueFiles) Type() string {
    return "valueFiles"
}

func (v *valueFiles) Set(value string) error {
    for _, filePath := range strings.Split(value, ",") {
        *v = append(*v, filePath)
    }
    return nil
}

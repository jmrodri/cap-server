package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"

	//"github.com/codeskyblue/go-sh"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

const MAIN_FILE = "Nulecule"

type Answers map[string]map[string]string

func runCommand(cmd string, args ...string) []byte {
	output, err := exec.Command(cmd, args...).CombinedOutput()
	if err != nil {
		fmt.Println("Error running " + cmd)
	}
	return output
}

// returns a map of maps
func parseBasicINI(data string) map[string]map[string]string {
	/*
		find first [ then find matching ]. Everything between them is the first key. Read until next [ or end of string.
	*/
	var answers = make(map[string]map[string]string)
	values := strings.SplitAfter(data, "\n")
	var key string
	for _, str := range values {
		if strings.HasPrefix(str, "[") {
			key = strings.Trim(str, "[]\n")
			answers[key] = make(map[string]string)
		} else {
			subvalue := strings.Split(str, " = ")
			answers[key][subvalue[0]] = strings.Trim(subvalue[1], "\n")
		}
	}

	fmt.Println(answers)
	return answers
}

func getAnswersFromFile(nulecule_path string) map[string]Answers {
	os.Remove("answers.conf")
	/*
		output, err := exec.Command("atomicapp", "genanswers", "nulecule-library/"+nulecule_path).CombinedOutput()
		if err != nil {
			fmt.Println("Error running atomicapp")
		}
	*/
	output := runCommand("atomicapp", "genanswers", "nulecule-library/"+nulecule_path)
	fmt.Println(string(output))
	answers, err := ioutil.ReadFile("answers.conf")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(answers))
	// add root node
	return map[string]Answers{"nulecule": parseBasicINI(string(answers))}
}

func getNuleculeList() map[string][]string {
	files, _ := ioutil.ReadDir("./nulecule-library")
	nulecules := make([]string, 0)
	for _, f := range files {
		if f.IsDir() {
			nulecules = append(nulecules, f.Name())
		}
	}
	return map[string][]string{"nulecules": nulecules}
}

func Nulecules(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(getNuleculeList())
}

func NuleculeDetails(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nulecule_id := vars["id"]
	json.NewEncoder(w).Encode(getAnswersFromFile(nulecule_id))
}

func NuleculeUpdate(w http.ResponseWriter, r *http.Request) {
	// update the nulecule answers file
	vars := mux.Vars(r)
	nulecule_id := vars["id"]
	fmt.Println(nulecule_id) // print it for now, will use for writing file
	fmt.Println("NuleculeUpdate!")
	res_map := make(map[string]interface{})
	res_map["foo"] = "bar"

	// ERIK TODO:
	// -> Convert answer JSON params -> map[string]interface{}
	// -> answerMap := addProviderDetails(map[string]interface{}) < adds provider necessary details to [general]
	// -> iniStruct := genINIFromAnswers(answerMap)
	// -> iniStruct.write(/* target nulecule directory */

	json.NewEncoder(w).Encode(res_map) // Success, fail?
}

func NuleculeDeploy(w http.ResponseWriter, r *http.Request) {
	/*
		// ZEUS TODO: call runCommand
		vars := mux.Vars(r)
		nulecule_id := vars["id"]
		output := runCommand("atomic", "run", "nulecule-library/"+nulecule_id)
		fmt.Println(string(output))
		json.NewEncoder(w).Encode(string(output))
	*/

	// ERIK TODO:
	// Very hardcoded currently.
	DEPLOY_SCRIPT := "/vagrant/examples/etherpad-centos7-atomicapp/run_etherpad.sh"

	// NOTE: MUST USE SCRIPT TO FAKE INTERACTIVE SHELL
	// script -c "$RUN_SCRIPT" /dev/null # Dump output file to dev null, we'll still have stdout for go
	cmd := exec.Command("script", "-c",
		fmt.Sprintf("\"%s\"", DEPLOY_SCRIPT),
		"/dev/null",
	)

	output, err := cmd.CombinedOutput()

	res_map := make(map[string]interface{})

	// NOTE: I think the error code might always be successful? Need to look at how the runscript
	// handles its exit codes.
	if err != nil {
		fmt.Println("ERROR!")
		fmt.Println(fmt.Sprint(err) + ": " + string(output))
		res_map["result"] = "failed"
	} else {
		fmt.Println(string(output))
		res_map["result"] = "successful"
	}

	json.NewEncoder(w).Encode(res_map)
}

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/nulecules", Nulecules)
	r.HandleFunc("/nulecules/{id}", NuleculeDetails).Methods("GET")
	r.HandleFunc("/nulecules/{id}", NuleculeUpdate).Methods("POST")
	r.HandleFunc("/nulecules/{id}/deploy", NuleculeDeploy).Methods("POST")
	fmt.Println("Listening on localhost:3001")

	allowed_headers := handlers.AllowedHeaders([]string{"Content-Type"})

	log.Fatal(http.ListenAndServe(":3001", handlers.CORS(
		allowed_headers,
	)(r)))
}

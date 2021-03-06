package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

// We need to strip namespaces off answer file sections to talk to the
// front end, but atmoicapp 0.6.4 expects them to be in the answerfile when
// we go to run it, so it will need to be written out to answer.conf.gen before
// running a user's answers. We'll keep track of that bookkeeping here
// TODO: Consider longer term approach than a gross global manifest!
var namespaceManifest NamespaceManifest

const MSG_BUFFER_SIZE = 20

func main() {
	namespaceManifest = make(NamespaceManifest)

	engine := NewWorkEngine(MSG_BUFFER_SIZE)
	subscriber := NewSocketWorkSubscriber()
	engine.AttachSubscriber(subscriber)

	http.Handle("/socket.io/", subscriber.Server)

	api := mux.NewRouter()
	api.HandleFunc("/api/nulecules", Nulecules).Methods("POST")
	api.HandleFunc("/api/nulecules/{registry}/{id}", NuleculeDetails).Methods("GET")
	api.HandleFunc("/api/nulecules/{registry}/{id}", NuleculeUpdate).Methods("POST")
	api.HandleFunc("/api/nulecules/{registry}/{id}/deploy",
		createEngineHandler(engine, NuleculeDeploy)).Methods("POST")

	allowedHeaders := handlers.AllowedHeaders([]string{"Content-Type"})
	http.Handle("/api/", handlers.CORS(allowedHeaders)(api))

	fs := http.StripPrefix("/static/", http.FileServer(http.Dir("./static")))
	http.Handle("/static/", fs)

	http.HandleFunc("/", IndexHandler)

	fmt.Println("Listening on localhost:3001")
	http.ListenAndServe(":3001", nil)
}

type GorillaRouteHandler func(http.ResponseWriter, *http.Request)
type EngineRouteHandler func(http.ResponseWriter, *http.Request, *WorkEngine)

func createEngineHandler(engine *WorkEngine, r EngineRouteHandler) GorillaRouteHandler {
	return func(writer http.ResponseWriter, request *http.Request) {
		r(writer, request, engine)
	}
}

func IndexHandler(w http.ResponseWriter, r *http.Request) {
	t, _ := template.ParseFiles("static/index.html")
	t.Execute(w, nil)
}

func Nulecules(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Entered Nulecules method")

	res_map := make(map[string]string)
	json.NewDecoder(r.Body).Decode(&res_map)

	organization := res_map["org"]
	username := res_map["username"]
	password := res_map["password"]

	channel := make(chan string)

	list := getNuleculeList(organization, username, password)
	responses := make([]string, len(list.Nulecules))

	filteredList := NuleculeList{}
	for _, nules := range list.Nulecules {
		go IsImageAtomicApp("docker://"+nules, channel)
	}

	counter := 1
	for response := range channel {
		responses = append(responses, response)
		if counter != len(list.Nulecules) {
			counter++
		} else {
			close(channel)
		}
	}

	for _, value := range responses {
		if strings.Compare(value, "ignore\n") != 0 && value != "" {
			// Strip docker:// protocol, 9 chars
			filteredList.Nulecules = append(filteredList.Nulecules, value[9:len(value)])
		}
	}

	json.NewEncoder(w).Encode(filteredList)

}

func NuleculeDetails(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Entered NuleculeDetails method")
	vars := mux.Vars(r)
	registry := vars["registry"]
	nuleculeId := vars["id"]

	downloadNulecule(registry, nuleculeId)

	// atomicapp 0.6.4 changed the answer.conf file format and namespaces
	// component names with their container name, i.e.
	// "mariadb-app" vs "mariadb-centos7-atomicapp:mariadb-app"
	// stripContainerNamespace will strip off the container namespace
	// to maintain backwards compatibility for the cap UI.

	strippedNamespaces, answers := stripNamespaces(
		getAnswersFromFile(registry, nuleculeId),
	)

	for _, strippedNamespace := range strippedNamespaces {
		namespaceManifest.insert(registry, nuleculeId,
			strippedNamespace.namespace, strippedNamespace.nodeName)
	}

	details := NuleculeDetail{
		answers,
		getBindings(registry, nuleculeId),
	}

	json.NewEncoder(os.Stdout).Encode(details)
	json.NewEncoder(w).Encode(details)
}

func NuleculeUpdate(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Entered NuleculeUpdate method")
	// update the nulecule answers file
	vars := mux.Vars(r)
	nuleculeId := vars["id"]
	registry := vars["registry"]

	// get the posted answers
	// Answers is a map of maps
	res_map := make(map[string]Answers)
	json.NewDecoder(r.Body).Decode(&res_map)

	// TODO: Consider better way to uniquely ID projects instead of a UUID
	// Could also use UUIDs as bookkeeping on the backend with a more friendly
	// project name provided by the user on the front end.
	projectName := addProviderDetails(res_map["nulecule"])
	createNewProject(projectName)
	injectNamespaces(namespaceManifest, res_map["nulecule"], registry, nuleculeId)
	writeUserAnswersToFile(registry, nuleculeId, res_map)

	json.NewEncoder(w).Encode(res_map) // Success, fail?
}

func NuleculeDeploy(w http.ResponseWriter, r *http.Request, engine *WorkEngine) {
	fmt.Println("Entered NuleculeDeploy method")
	vars := mux.Vars(r)
	nuleculeId := vars["id"]
	registry := vars["registry"]

	// TODO: probably a much better way to get the host
	answers := getGeneratedAnswersFile(registry, nuleculeId)
	entry_map := findEntry(answers, "endpoint")
	jobToken := engine.StartNewJob(NewDeployJob(registry, nuleculeId, entry_map["endpoint"]))

	// TODO: Error handling!
	res_map := make(map[string]interface{})
	res_map["job_token"] = jobToken
	res_map["result"] = "success"

	json.NewEncoder(w).Encode(res_map) // Success, fail?
}

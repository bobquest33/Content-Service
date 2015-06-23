package ContentService

import (
  "github.com/gorilla/mux"

  "net/http"
  "mime/multipart"
  "encoding/json"
  "errors"
  "fmt"
  "strings"
  "os"
  "path"
)

type ContentService struct {
  r *mux.Router
  mongo *MongoDBBackend
}

var ORIG_PATH string

func Create(originalsPath string, mongoURL string, mongoDBName string) (service *ContentService, err error){
  ORIG_PATH = originalsPath

  // Create folder for pictures
  err = os.MkdirAll(path.Join(ORIG_PATH), 0777)
  if err != nil {
    return service, err
  }

  // Creating the service to store user content
  service = &ContentService{}

  service.r = mux.NewRouter()
  if err != nil {
    return service, err
  }

  service.r.HandleFunc("/{itemId}/{pictureType}", service.HandleUploadPicture).Methods("POST")
  service.r.HandleFunc("/{itemId}/{pictureType}/{pictureName}", service.HandlDeletePicture).Methods("DELETE")
  service.r.HandleFunc("/{itemId}/{pictureType}/{pictureName}", service.HandlePictureConfirmation).Methods("PUT")

  service.r.PathPrefix("/").Handler(http.FileServer(http.Dir(ORIG_PATH))).Methods("GET")

  service.mongo, err = CreateMongoBackend(mongoURL, mongoDBName)

  return service, err
}

func (service *ContentService) Start(port int) error{
  http.Handle("/", service.r)
  fmt.Printf("Pictures Server running on port %d\n", port)
  http.ListenAndServe(fmt.Sprintf(":%d", port), nil)

  return nil
}

func (service *ContentService) HandleUploadPicture(rw http.ResponseWriter, req *http.Request) {
  var err error

  vars := mux.Vars(req)
  itemId := vars["itemId"]
  pictureType := vars["pictureType"]

  if !service.Supports(pictureType) {
    HandleError(404, errors.New("Not found"), rw)
    return
  }

  err = service.Authorize(req)
  if err != nil {
    HandleError(403, err, rw)
    return
  }

  var itemType string
  var picturePath string

  itemType, err = service.mongo.GetItemType(itemId)
  if err != nil {
    HandleError(500, err, rw)
  }

  fmt.Println(itemType)
  if itemType == TEMP_TYPE{
    itemId = "temp"
  }

  err = os.MkdirAll(path.Join(ORIG_PATH, itemId), 0777)
  if err != nil {
    HandleError(500, err, rw)
    return
  }

  var (
    file multipart.File
    fileHeader *multipart.FileHeader
  )

  file, fileHeader, err = req.FormFile("picture")
  defer file.Close()
  if err != nil {
    HandleError(500, err, rw)
    return
  }

  var filename string
  filename, err = service.UploadPicture(file, fileHeader, itemId, pictureType)
  if err != nil {
    HandleError(500, err, rw)
    return
  }

  rw.WriteHeader(200)
  rw.Write([]byte(filename))
}

func HandlDeletePicture(rw http.ResponseWriter, req *http.Request) {
  var err error

  vars := mux.Vars(req)
  itemId := vars["itemId"]
  pictureType := vars["pictureType"]
  pictureName := vars["pictureName"]

  err = service.RemovePicture(itemId, pictureType, pictureName)
  if err != nil {
    HandleError(500, err, rw)
    return
  }

  rw.WriteHeader(200)
}

func HandlePictureConfirmation(rw http.ResponseWriter, req *http.Request) {
  var err error

  vars := mux.Vars(req)
  itemId := vars["itemId"]
  pictureType := vars["pictureType"]
  pictureName := vars["pictureName"]

  err = service.ConfirmPicture(itemId, pictureType, pictureName)
  if err != nil {
    HandleError(500, err, rw)
    return
  }

  rw.WriteHeader(200)
}

func HandleError(code int, err error, rw http.ResponseWriter) {
  rw.WriteHeader(code)
  rw.Write([]byte(err.Error()))
}

func (service *ContentService) Authorize(req *http.Request) error {
  var err error
  var sessionCookie *http.Cookie
  var sessionId string

  sessionCookie, err = req.Cookie("connect.sid")
  if err != nil {
    return err
  }
  sessionId = strings.Split(sessionCookie.Value, ".")[0]
  sessionId = sessionId[4:len(sessionId)]

  c := service.mongo.C("sessions")
  var sessionStruct *SessionModel

  sessionStruct, err = service.mongo.FindSessionById(sessionId)
  if err != nil {
    return err
  }

  var session map[string]map[string]interface{}

  err = json.Unmarshal([]byte(sessionStruct.Session), &session)
  if err != nil {
    return err
  }

  userId := session["passport"]["user"].(string)
  var count int

  count, err = service.mongo.UsersCount(userId)
  if err != nil {
    return err
  }
  if count == 0 {
    return errors.New("Not authorized")
  }

  return nil
}

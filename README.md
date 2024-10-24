## Requirements
Add a `compose.yaml` file at the root of the repository and replace the XXX with your environment variable values.
```
version: '3'
services:
  minio:
    image: quay.io/minio/minio
    command:
      - server
      - /data
      - --console-address
      - :9001
    ports:
      - "9000:9000"
      - "9001:9001" 
  
  web:
    build: .
    ports:
      - "8080:8080"
    depends_on:
      - minio 
    mem_limit: 32m
    environment:
      SYM_KEY: XXX
      MINIO_USER: XXX
      MINIO_PWD: XXX
```

## How To Run
`docker-compose up --build` from the root directory, where the `Dockerfile` and `compose.yaml` files are.

## API
<ul>
<li><strong>localhost:8080/upload</strong> used to upload files provided in a <strong>POST</strong> request  

#### Parameters:

- **_Mandatory:_** `file`  
  The file to be uploaded, with the part name `"file"`.
  
- **_Mandatory:_** `File-Size`  
  A header field representing the file size in bytes.
  
- **_Optional:_** `Uid`  
  A header field containing a `uint64` value that represents the UID you'd like to store the file under.  
  If the UID is already in use, the request will fail, but an available UID will be recommended.  
  If the `Uid` header is not provided, the system will assign a UID and return it after the file is uploaded, so you can use it to retrieve the file later.

</li>
<li><strong>localhost:8080/fetch?uid=fileNbr</strong> used to download the file using a <strong>GET</strong> request.</li>  

#### Parameters:

- **_Mandatory:_** `uid`  
  The URL parameter, telling the server which file to fetch. If the uid is not mapped to any file, the request will fail.
</ul>

## Examples
To upload a file, you can try:
```
curl -H "File-Size: 497" -H "Uid: 1" -X POST http://localhost:8080/upload \ 
     -F "file=@/path/to/script.sh"
```
or without providing a Uid:
```
curl -H "File-Size: 3200000" -X POST http://localhost:8080/upload \ 
     -F "file=@/path/to/image.jpg"
```

these requests will return something such as:

```
File successfully uploaded and encrypted with UID 393.
```

and you can fetch any file by running

```
curl -OJ "http://localhost:8080/fetch?uid=393"
```

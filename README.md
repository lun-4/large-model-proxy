# Large Model Proxy

Large Model Proxy is designed to make it easy to run multiple resource-heavy Large Models (LM) on the same machine with limited amount of VRAM/other resources.
 It listens on a dedicated port for each proxied LM and/or a single port for OpenAI API, making LMs always available to the clients connecting to these ports.


## How it works

Upon receiving a connection, if LM on this port or `model` specified in JSON payload to an OpenAI API endpoint is not already started, it will:

1. Verify if the required resources are available to start the corresponding LM.
2. If resources are not available, it will automatically stop the least recently used LM to free up contested resources.
3. Start LM.
4. Wait for LM to be available on the specified port.
5. Wait for healthcheck to pass. 

Then it will proxy the connection from the LM to the client that connected to it.
To the client this should be fully transparent with the only exception being that receiving any data on the connection takes longer if LM had to be spun up. 

## Installation
**Ubuntu and Debian**: Download the deb file attached to the [latest release](https://github.com/perk11/large-model-proxy/releases/latest).

**Arch Linux**: Install from [AUR](https://aur.archlinux.org/packages/large-model-proxy).

**Other Distros**:
1. Install go

2. Clone the repository:
    ```sh
    git clone https://github.com/perk11/large-model-proxy.git
    ```
3. Navigate into the project directory:
    ```sh
    cd large-model-proxy
    ```
4. Build the project:
    ```sh
    go build -o large-model-proxy main.go
    ```
    or
   ```sh
    make
    ```
**Windows**: Not currently tested, but should work in WSL using "Ubuntu" or "Other Distros" instruction.
It will probably not work on Windows natively as it is using Unix Process Groups.

**macOS**: Might work using "Other Distros" instruction, but I do not own any Apple devices to check, please let me know if it does!

## Configuration

Below is an example config.json:
```json
{
   "OpenAiApi": {
      "ListenPort": "7070"
   }, 
  "MaxTimeToWaitForServiceToCloseConnectionBeforeGivingUpSeconds": 1200,
  "ShutDownAfterInactivitySeconds": 120,
  "ResourcesAvailable": {
     "VRAM-GPU-1": 24000,
     "RAM": 32000
  }, 
  "Services": [
    {
      "Name": "automatic1111",
      "ListenPort": "7860",
      "ProxyTargetHost": "localhost",
      "ProxyTargetPort": "17860",
      "Command": "/opt/stable-diffusion-webui/webui.sh",
      "Args": "--port 17860",
      "WorkDir": "/opt/stable-diffusion-webui", 
      "ShutDownAfterInactivitySeconds": 600,
      "RestartOnConnectionFailure": true,
      "ResourceRequirements": {
        "VRAM-GPU-1": 2000,
        "RAM": 30000
      }
    },
    {
      "Name": "Gemma-27B",
      "OpenAiApi": true,
      "ListenPort": "8081",
      "ProxyTargetHost": "localhost",
      "ProxyTargetPort": "18081",
      "Command": "/opt/llama.cpp/llama-server",
      "Args": "-m /opt/Gemma-27B-v1_Q4km.gguf -c 8192 -ngl 100 -t 4 --port 18081",
      "HealthcheckCommand": "curl --fail http://localhost:18081/health", 
      "HealthcheckIntervalMilliseconds": 200,
      "RestartOnConnectionFailure": false,
      "ResourceRequirements": {
        "VRAM-GPU-1": 20000,
        "RAM": 3000
      }
    },
    {
        "Name": "Qwen/Qwen2.5-7B-Instruct",
        "OpenAiApi": true,
        "ProxyTargetHost": "localhost",
        "ProxyTargetPort": "18082",
        "Command": "/home/user/.conda/envs/vllm/bin/vllm",
        "LogFilePath": "/var/log/Qwen2.5-7B.log",
        "Args": "serve Qwen/Qwen2.5-7B-Instruct --port 18082",
        "ResourceRequirements": {
           "VRAM-GPU-1": 17916
        }
     }
  ]
}
```
Bellow is a breakdown of what this configuration does:

1. Any client can access the following services:
   * Automatic1111's Stable Diffusion web UI on port 7860
   * llama.cpp with Gemma2 on port 8081
   * OpenAI API on port 7070, supporting Gemma2 via llama.cpp and Qwen2.5-7B-Instruct via vLLM, depending on the `model` specified in the JSON payload.
2. Internally large-model-proxy will expect Automatic1111 to be available on port 17860, Gemma27B on port 18081 and Qwen2.5-7B-Instruct on port 18082 once it runs the commands given in "Command" parameter and healthcheck passes. 
3. This config allocates up to 24GB of VRAM and 32GB of RAM for them. No more GPU or RAM will be attempted to be used (assuming the values in ResourceRequirements are correct).
4. The Stable Diffusion web UI is expected to use up to 3GB of VRAM and 30GB of RAM, while Gemma27B will use up to 20GB of VRAM and 3GB of RAM and Qwen2.5-7B-Instruct up to 18GB of VRAM and no RAM (for example's sake).
5. Automatic111 and Gemma2 logs will be in logs/ directory of the current dir, while Qwen logs will be in /var/log/Qwen.log 

Note how Qwen is not available directly, but is only available via OpenAI API.

With this configuration Qwen and Automatic111 can run at the same times. Assuming they do, a request for  Gemma will unload the one least recently used. If they are currently in use, a request to Gemma will have to wait for one of the other services to free up.
"ResourcesAvailable" can include any resource metrics, CPU cores, multiple VRAM values for multiple GPUs, etc. these values are not checked against actual usage.

## Usage
```sh
./large-model-proxy -c path/to/config.json
```

If `-c` argument is omitted, `large-model-proxy` will look for `config.json` in current directory

## OpenAI API endpoints

Currently, the following OpenAI API endpoints are supported:
* `/v1/completions`
* `/v1/chat/completions`
* `/v1/models` (This one makes it work with e.g. Open WebUI seamlessly).
* More to come

## Logs

Output from each service is logged to a separate file. Default behavior is to log it into logs/{name}.log,
but it can be redefined by specifying `LogFilePath` parameter for each service.

## Contacts

Please join my Telegram group for any feedback, questions and collaboration:
https://t.me/large_model_proxy
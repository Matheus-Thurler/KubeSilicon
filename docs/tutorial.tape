# Records media/kubesilicon-tutorial.mp4 (1920x1080) from a clean slate. Run from the repo root:
#   vhs docs/tutorial.tape
# Needs: vhs, Docker Desktop, go, kind, kubectl, qemu, a demo key in ~/.kubesilicon/demo-key,
# and the Ubuntu image already cached in ~/.kubesilicon/cache (the first run downloads it).
Output media/kubesilicon-tutorial.mp4
Set Shell "bash"
Set FontSize 26
Set Width 1920
Set Height 1080
Set Padding 40
Set Framerate 30
Set Theme "Catppuccin Mocha"
Set TypingSpeed 30ms

Hide
Type `export PS1='\[\e[1;32m\]$\[\e[0m\] '` Enter
Type `cd ~/Documents/estudo/kubesilicon && export PATH=$PWD/bin:$PATH` Enter
Type `kind delete cluster --name kubesilicon >/dev/null 2>&1; make -s uninstall >/dev/null 2>&1; rm -rf bin ~/.kubesilicon/vms ~/.kubesilicon/kubesilicond.log` Enter
Type `eval "$(ssh-agent -s)" >/dev/null; ssh-add -q ~/.kubesilicon/demo-key` Enter
Sleep 20s
Type "clear" Enter
Show

Type "# KubeSilicon: run Linux VMs from Kubernetes objects on your Mac" Enter
Sleep 2s
Type "# This tutorial starts from nothing and ends with a VM you can ssh into." Enter
Sleep 3s
Type "# You need a Mac with Apple Silicon and these tools:" Enter
Type "command -v docker kind kubectl go qemu-img" Enter
Sleep 5s

Ctrl+L
Type "# Step 1: a local Kubernetes cluster (kind runs it inside Docker)" Enter
Type "kind create cluster --name kubesilicon" Enter
Wait+Screen@240s /You can now use your cluster/
Sleep 2s
Type "kubectl get nodes" Enter
Sleep 4s

Ctrl+L
Type "# Step 2: install KubeSilicon. This does three things:" Enter
Type "#  - builds the daemon, the program that creates the VMs" Enter
Type "#  - teaches Kubernetes a new object type, SiliconVM" Enter
Type "#  - starts the daemon in the background on your Mac (it runs outside the cluster)" Enter
Sleep 5s
Type "make install CTX=kind-kubesilicon" Enter
Wait+Screen@240s /daemon running/
Sleep 4s
Type "# Check: the daemon is running and Kubernetes knows the new type" Enter
Type "tail -1 ~/.kubesilicon/kubesilicond.log; kubectl get crd | grep kubesilicon" Enter
Sleep 6s

Ctrl+L
Type "# Step 3: describe a VM in YAML, like any Kubernetes object" Enter
Type "cat examples/vm.yaml" Enter
Sleep 14s
Type "# The VM trusts an SSH public key. Here a demo key; use your own ~/.ssh/id_ed25519.pub" Enter
Type `export SSH_PUBKEY="$(cat ~/.kubesilicon/demo-key.pub)"` Enter
Sleep 4s

Ctrl+L
Type "# Step 4: create the VM and watch it boot" Enter
Type "envsubst < examples/vm.yaml | kubectl apply -f -" Enter
Sleep 2s
Type "kubectl get svm demo -w" Enter
Wait+Screen@240s /Running/
Sleep 3s
Ctrl+C
Sleep 1s
Type "# Running, with an IP. Give the guest a few seconds to finish booting:" Enter
Type "export KUBESILICON_CONTEXT=kind-kubesilicon" Enter
Type "until kubesilicon ssh demo -- true 2>/dev/null; do sleep 2; done; echo ready" Enter
Wait+Screen@120s /ready/
Sleep 2s

Ctrl+L
Type "# Step 5: run commands inside the VM over SSH" Enter
Type `kubesilicon ssh demo -- 'hostname; uname -srm; free -h | head -2'` Enter
Sleep 8s
Type "# For an interactive shell, drop the command: kubesilicon ssh demo" Enter
Sleep 4s

Ctrl+L
Type "# Step 6: the VM is reachable from pods, by name" Enter
Type "kubectl run probe --rm -i --restart=Never --image=busybox:1.36 -- ping -c2 demo.default.svc.cluster.local 2>&1 | tail -4" Enter
Wait+Screen@120s /deleted/
Sleep 5s

Ctrl+L
Type "# Step 7: stop and start. The disk is kept." Enter
Type `kubectl patch svm demo --type=merge -p '{"spec":{"running":false}}'` Enter
Type `until [ "$(kubectl get svm demo -o jsonpath={.status.phase})" = Stopped ]; do sleep 1; done; kubectl get svm demo` Enter
Wait+Screen@120s /Stopped +2/
Sleep 3s
Type `kubectl patch svm demo --type=merge -p '{"spec":{"running":true}}'` Enter
Type `until [ "$(kubectl get svm demo -o jsonpath={.status.phase})" = Running ]; do sleep 1; done; kubectl get svm demo` Enter
Wait+Screen@120s /Running +[0-9]/
Sleep 4s

Ctrl+L
Type "# Cleanup" Enter
Type "kubectl delete svm demo" Enter
Sleep 3s
Wait+Line@180s /\$$/
Type "make uninstall" Enter
Sleep 4s
Wait+Line@60s /\$$/
Type "# To remove the cluster too: kind delete cluster --name kubesilicon" Enter
Sleep 4s
Type "# Docs: github.com/Matheus-Thurler/KubeSilicon" Enter
Sleep 4s

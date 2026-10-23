# Records media/kubesilicon-demo.mp4 (1920x1080). Run from the repo root: vhs docs/demo.tape
# Needs: vhs, a kind cluster with the "kind-kubesilicon" kubeconfig context, the CRD applied,
# bin/kubesilicond and bin/kubesilicon built, and a demo key in ~/.kubesilicon/demo-key.
Output media/kubesilicon-demo.mp4
Set Shell "bash"
Set FontSize 26
Set Width 1920
Set Height 1080
Set Padding 40
Set Framerate 30
Set Theme "Catppuccin Mocha"
Set TypingSpeed 25ms

Hide
Type `export PS1='\[\e[1;32m\]kubesilicon\[\e[0m\] $ '` Enter
Type `cd ~/Documents/estudo/kubesilicon && export PATH=$PWD/bin:$PATH` Enter
Type `export KUBESILICON_CONTEXT=kind-kubesilicon SSH_PUBKEY="$(cat ~/.kubesilicon/demo-key.pub)"` Enter
Type `eval "$(ssh-agent -s)" >/dev/null; ssh-add -q ~/.kubesilicon/demo-key` Enter
Type `alias k='kubectl --context kind-kubesilicon'` Enter
Type `make -s bin/kubesilicond bin/kubesilicon` Enter
Sleep 3s
Type "clear" Enter
Show

# 1 -----------------------------------------------------------------------
Type "# KubeSilicon: Linux VMs from Kubernetes objects on Apple Silicon, KubeVirt-style" Enter
Type "# 1. This Mac and the kind cluster already running on it" Enter
Type "sysctl -n machdep.cpu.brand_string; k get nodes" Enter
Sleep 4s
Type "# 2. The SiliconVM CRD is installed (group kubesilicon.io)" Enter
Type "k get crd siliconvms.kubesilicon.io" Enter
Sleep 4s

# 2 -----------------------------------------------------------------------
Ctrl+L
Type "# 3. kubesilicond runs outside the cluster, as a LaunchAgent" Enter
Type "hack/agent.sh install kubesilicon 2>&1 | tail -1" Enter
Sleep 6s
Type "tail -2 ~/.kubesilicon/kubesilicond.log" Enter
Sleep 4s

# 3 -----------------------------------------------------------------------
Ctrl+L
Type "# 4. A VM is a Kubernetes object" Enter
Type "cat examples/vm.yaml" Enter
Sleep 8s
Type "envsubst < examples/vm.yaml | k apply -f -" Enter
Sleep 1s
Type "k get svm demo -w" Enter
Sleep 30s
Ctrl+C
Sleep 2s

# 4 -----------------------------------------------------------------------
Ctrl+L
Type "# 5. The kubesilicon client: ip and ssh (first wait for the guest sshd)" Enter
Type "kubesilicon ip demo" Enter
Sleep 2s
Type `until printf "" | nc -w2 $(kubesilicon ip demo) 22 | grep -q SSH; do sleep 2; done; echo sshd is up` Enter
Sleep 8s
Type `kubesilicon ssh demo -- 'hostname; uname -srm; nproc; free -m | sed -n 2p; df -h / | tail -1'` Enter
Sleep 6s

# 5 -----------------------------------------------------------------------
Ctrl+L
Type "# 6. Every VM gets a DNS name inside the cluster" Enter
Type "k get svc,endpointslice demo" Enter
Sleep 4s
Type `k run probe --rm -i --restart=Never --image=busybox:1.36 -- sh -c 'nslookup demo.kubesilicon.svc.cluster.local | tail -3; ping -c2 demo.kubesilicon.svc.cluster.local | tail -3; nc -zv -w3 demo.kubesilicon.svc.cluster.local 22' 2>&1 | grep -v 'All commands\|If you don'` Enter
Sleep 20s

# 6 -----------------------------------------------------------------------
Ctrl+L
Type "# 7. Serial console (Ctrl-] to leave)" Enter
Type "kubesilicon console demo" Enter
Sleep 2s
Enter
Sleep 3s
Type "ubuntu" Enter
Sleep 2s
Type "demo" Enter
Sleep 5s
Type "whoami; uname -r" Enter
Sleep 3s
Ctrl+]
Sleep 2s

# 7 -----------------------------------------------------------------------
Ctrl+L
Type "# 8. Stop and start: running=false keeps the disk and clears the DNS endpoint" Enter
Type `k patch svm demo --type=merge -p '{"spec":{"running":false}}'` Enter
Sleep 2s
Type `until [ "$(k get svm demo -o jsonpath={.status.phase})" = Stopped ]; do sleep 1; done; k get svm demo; k get endpointslice demo` Enter
Sleep 35s
Type `k patch svm demo --type=merge -p '{"spec":{"running":true}}'` Enter
Sleep 2s
Type `until [ "$(k get svm demo -o jsonpath={.status.phase})" = Running ]; do sleep 1; done; k get svm demo` Enter
Sleep 25s

# 8 -----------------------------------------------------------------------
Ctrl+L
Type "# 9. VMs live in their own process: killing the daemon does not touch them" Enter
Type `ps -axo pid,command | grep '[k]ubesilicond' | cut -c1-100` Enter
Sleep 4s
Type `pkill -9 -f '.kubesilicon/bin/kubesilicond -context'` Enter
Sleep 1s
Type `kubesilicon ssh demo -- 'uptime'` Enter
Sleep 5s
Type "# launchd brings the daemon back, and it adopts the running VM" Enter
Sleep 12s
Type `ps -axo pid,command | grep '[k]ubesilicond' | cut -c1-100` Enter
Sleep 3s
Type "k get svm demo" Enter
Sleep 4s

# 9 -----------------------------------------------------------------------
Ctrl+L
Type "# 10. Delete: the VM, its disk, and its DNS objects go away" Enter
Type "k delete svm demo" Enter
Sleep 30s
Type "ls ~/.kubesilicon/vms | wc -l; k get svc demo" Enter
Sleep 3s
Type `ps -axo pid,command | grep '[k]ubesilicond launch' || echo 'no VM process left'` Enter
Sleep 3s
Type "hack/agent.sh uninstall" Enter
Sleep 3s
Type "# github.com/Matheus-Thurler/KubeSilicon" Enter
Sleep 4s

Hide
Type `eval "$(ssh-agent -k)" >/dev/null` Enter

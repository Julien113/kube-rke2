package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func TestLivenessProbeRestartsContainer(t *testing.T) {
	// 1. CONNEXION AU CLUSTER
	home := homedir.HomeDir()
	kubeconfigPath := filepath.Join(home, ".kube", "config")
	clientConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		t.Fatalf(" [ÉCHEC] Erreur kubeconfig : %v", err)
	}

	clientset, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		t.Fatalf(" [ÉCHEC] Erreur création client K8s : %v", err)
	}

	namespaceName := "test-auto"
	podName := "liveness-failure-test"
	ctx := context.Background()

	// 2. PRÉPARATION DU NAMESPACE
	nsSpec := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName}}
	_, _ = clientset.CoreV1().Namespaces().Create(ctx, nsSpec, metav1.CreateOptions{})

	// Nettoyage automatique du Pod à la fin du test
	defer func() {
		t.Logf(" [TEARDOWN] Nettoyage : Suppression du Pod '%s'...", podName)
		deleteCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = clientset.CoreV1().Pods(namespaceName).Delete(deleteCtx, podName, metav1.DeleteOptions{})
	}()

	// 3. DÉPLOIEMENT D'UN POD QUI DEVIENT "SICK" (MALADE) AU BOUT DE 10 SECONDES
	// Le conteneur crée /tmp/healthy au démarrage puis le supprime après 10s.
	t.Logf(" [SETUP] Déploiement du Pod '%s' configuré pour échouer après 10s...", podName)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespaceName,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "liveness-demo",
					Image: "busybox:latest",
					// Script : crée /tmp/healthy, attend quelques secondes, puis supprime le fichier
					Command: []string{
						"/bin/sh",
						"-c",
						"touch /tmp/healthy && sleep 1 && rm -f /tmp/healthy && sleep 600",
					},
					// Sonde Liveness : vérifie l'existence du fichier /tmp/healthy
					LivenessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							Exec: &corev1.ExecAction{
								Command: []string{"cat", "/tmp/healthy"},
							},
						},
						InitialDelaySeconds: 1, // Attend 1s avant de commencer à tester
						PeriodSeconds:       1, // Teste toutes les 1s
						FailureThreshold:    1, // 1 seul échec suffit pour déclencher le redémarrage
					},
				},
			},
		},
	}

	_, err = clientset.CoreV1().Pods(namespaceName).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf(" [ÉCHEC] Impossible de créer le Pod : %v", err)
	}

	// 4. ATTENTE DU PREMIER DÉMARRAGE (RESTARTCOUNT == 0)
	t.Log(" [STEP 1] Attente du démarrage initial du conteneur...")
	time.Sleep(5 * time.Second)

	initialPod, err := clientset.CoreV1().Pods(namespaceName).Get(ctx, podName, metav1.GetOptions{})
	if err == nil && len(initialPod.Status.ContainerStatuses) > 0 {
		t.Logf("   -> Statut initial : Restarts = %d", initialPod.Status.ContainerStatuses[0].RestartCount)
	}

	// 5. ATTENTE DU REDÉMARRAGE FORCÉ PAR LA LIVENESS PROBE (RESTARTCOUNT >= 1)
	t.Log(" [STEP 2] Attente que la Liveness Probe échoue et force le redémarrage (Timeout: 25s)...")

	restartDetected := false
	maxAttempts := 35
	pollInterval := 1 * time.Second

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		currentPod, err := clientset.CoreV1().Pods(namespaceName).Get(ctx, podName, metav1.GetOptions{})
		if err == nil && len(currentPod.Status.ContainerStatuses) > 0 {
			restartCount := currentPod.Status.ContainerStatuses[0].RestartCount

			t.Logf("   -> Tentative %d/%d : RestartCount = %d", attempt, maxAttempts, restartCount)

			// Si le nombre de redémarrages est supérieur à 0, Kubernetes a bien tué et relancé le conteneur !
			if restartCount > 0 {
				restartDetected = true
				break
			}
		}

		time.Sleep(pollInterval)
	}


	// 6. Verification du redémarrage du pod à running 
	
	readinessPassed := false
	maxAttempts = 20
	pollInterval = 2 * time.Second

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Récupération de l'état en temps réel du Pod
		currentPod, err := clientset.CoreV1().Pods(namespaceName).Get(ctx, podName, metav1.GetOptions{})
		if err == nil && len(currentPod.Status.ContainerStatuses) > 0 {
			
			// On inspecte le statut du conteneur principal
			containerStatus := currentPod.Status.ContainerStatuses[0]
			t.Logf("   -> Tentative %d/%d : Conteneur '%s' | Phase = %s | Ready = %t", 
				attempt, maxAttempts, containerStatus.Name, currentPod.Status.Phase, containerStatus.Ready)

			// Si le champ Ready passe à true, la sonde a validé le conteneur !
			if containerStatus.Ready {
				readinessPassed = true
				break
			}
		}

		time.Sleep(pollInterval)
	}

	// 6. ASSERTION FINALE
	if !restartDetected {
		t.Fatalf(" [ÉCHEC] La Liveness Probe n'a pas déclenché de redémarrage du conteneur !")
	}
	if !readinessPassed {
		t.Fatalf(" [ÉCHEC] Le conteneur n'est pas passé à l'état READY après le redémarrage !")
	}	

	t.Logf(" [SUCCÈS] La Liveness Probe a détecté la panne et Kubernetes a redémarré le conteneur avec succès !")
}
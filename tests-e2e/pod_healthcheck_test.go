package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)


func TestNginxLifecycleInTestAuto(t *testing.T) {
	// =========================================================================
	// 1. INITIALISATION ET CONNEXION AU CLUSTER
	// =========================================================================
	home := homedir.HomeDir()
	if home == "" {
		t.Fatalf(" [ERREUR] Impossible de localiser le dossier $HOME.")
	}

	kubeconfigPath := filepath.Join(home, ".kube", "config")
	clientConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		t.Fatalf(" [ÉCHEC] Erreur chargement kubeconfig : %v", err)
	}

	clientset, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		t.Fatalf(" [ÉCHEC] Erreur création du client K8s : %v", err)
	}

	// Définition des variables de test
	namespaceName := "test-auto"
	podName := "nginx-test"
	
	// Utilisation de context.Background() car la boucle de polling gèrera son propre timeout.
	ctx := context.Background()

	// =========================================================================
	// 2. ÉTAPE 2 : DÉPLOIEMENT DU POD NGINX
	// =========================================================================
	t.Logf(" [SETUP] Déploiement du Pod '%s' dans le namespace '%s'...", podName, namespaceName)

	nginxPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespaceName,
			Labels: map[string]string{
				"app": "nginx-e2e",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:latest", // Image Nginx officielle
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 80,
						},
					},
					// ---------------------------------------------------------
					// SONDE LIVENESS : Redémarre le conteneur s'il ne répond pas
					// ---------------------------------------------------------
					LivenessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/",
								Port: intstr.FromInt(80),
							},
						},
						InitialDelaySeconds: 5,  // Attendre 5s avant le 1er test
						PeriodSeconds:       10, // Tester toutes les 10s
					},
					// ---------------------------------------------------------
					// SONDE READINESS : Bloque le trafic tant qu'il n'est pas prêt
					// ---------------------------------------------------------
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/",
								Port: intstr.FromInt(80),
							},
						},
						InitialDelaySeconds: 2, // Attendre 2s avant le 1er test
						PeriodSeconds:       5, // Tester toutes les 5s
					},
				},
			},
		},
	}

	// Création du Pod dans Kubernetes
	createdPod, err := clientset.CoreV1().Pods(namespaceName).Create(ctx, nginxPod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf(" [ÉCHEC] Impossible de créer le Pod : %v", err)
	}

	// Nettoyage automatique du Pod à la fin du test
	defer func() {
		t.Logf(" [TEARDOWN] Nettoyage : Suppression du Pod '%s'...", podName)
		deleteCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = clientset.CoreV1().Pods(namespaceName).Delete(deleteCtx, podName, metav1.DeleteOptions{})
	}()

	// =========================================================================
	// 4. ASSERTION : VÉRIFICATION DE LA PRÉSENCE DES PROBES
	// =========================================================================
	t.Log(" [VALIDATION] Inspection de la configuration des sondes sur le Pod...")

	// Récupération de la spécification du premier conteneur
	if len(createdPod.Spec.Containers) == 0 {
		t.Fatalf(" [ÉCHEC] Aucun conteneur trouvé dans la spécification du Pod.")
	}

	mainContainer := createdPod.Spec.Containers[0]

	// 1. Vérification de la Liveness Probe
	if mainContainer.LivenessProbe == nil {
		t.Fatalf(" [ÉCHEC VALIDATION] La Liveness Probe est absente (nil) sur le conteneur '%s' !", mainContainer.Name)
	} else {
		t.Logf(" [OK] Liveness Probe détectée (HTTP GET sur port 80)")
	}

	// 2. Vérification de la Readiness Probe
	if mainContainer.ReadinessProbe == nil {
		t.Fatalf(" [ÉCHEC VALIDATION] La Readiness Probe est absente (nil) sur le conteneur '%s' !", mainContainer.Name)
	} else {
		t.Logf(" [OK] Readiness Probe détectée (HTTP GET sur port 80)")
	}

	t.Logf(" [SUCCÈS PARFAIT] Le Pod '%s' possède bien ses deux sondes configurées !", podName)

	// 4. ATTENTE ET VÉRIFICATION DE L'ÉTAT READINESS
	t.Log(" [WAIT] Attente de la validation de la Readiness Probe par Kubernetes...")

	readinessPassed := false
	maxAttempts := 20
	pollInterval := 2 * time.Second

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

	// 5. ASSERTION FINALE
	if !readinessPassed {
		t.Fatalf(" [ÉCHEC] La Readiness Probe n'est pas passée à Ready après %v !", time.Duration(maxAttempts)*pollInterval)
	}

	t.Logf(" [SUCCÈS] La Readiness Probe a réussi ! Le conteneur est marqué READY dans Kubernetes.")

}
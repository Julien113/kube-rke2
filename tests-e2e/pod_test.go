package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	corev1 "k8s.io/api/core/v1"
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
				},
			},
		},
	}

	// Création du Pod dans le cluster (kubectl apply / run)
	_, err = clientset.CoreV1().Pods(namespaceName).Create(ctx, nginxPod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf(" [ÉCHEC] Impossible de créer le Pod Nginx : %v", err)
	}
	t.Logf(" [OK] Ordre de création du Pod transmis au cluster.")

	// =========================================================================
	// 3. ÉTAPE 3 : TEARDOWN / NETTOYAGE DYNAMIQUE (SUPPRESSION DU POD)
	// =========================================================================
	// Le bloc defer s'exécutera TOUJOURS à la fin de la fonction,
	// garantissant la suppression du Pod, même si l'assertion échoue plus bas.
	defer func() {
		t.Logf(" [TEARDOWN] Nettoyage : Suppression du Pod '%s'...", podName)
		
		deleteTimeoutCtx, cancelDelete := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelDelete()

		// Suppression effective du Pod (kubectl delete pod nginx-test -n test-auto)
		err := clientset.CoreV1().Pods(namespaceName).Delete(deleteTimeoutCtx, podName, metav1.DeleteOptions{})
		if err != nil {
			t.Errorf(" [AVERTISSEMENT] Échec de la suppression du Pod lors du nettoyage : %v", err)
		} else {
			t.Logf(" [SUCCÈS TEARDOWN] Le Pod '%s' a été correctement supprimé.", podName)
		}
	}()

	
	// =========================================================================
	// 4. ÉTAPE 4 : LISTE DES PODS DANS LE NAMESPACE 'test-auto'
	// =========================================================================
	list, err2 := clientset.CoreV1().Pods("test-auto").List(ctx, metav1.ListOptions{})
	if err2 != nil {
		t.Fatalf("Impossible de lister les pods : %v", err2)
	}
	t.Logf("Nombre de pods trouvés : %d", len(list.Items))
	for _, pod := range list.Items {
		t.Logf("Pod trouvé : %s", pod.Name)
	}

	// =========================================================================
	// 5. ÉTAPE 5 : VÉRIFICATION QUE LE POD PASSE À L'ÉTAT 'RUNNING' (POLLING)
	// =========================================================================
	// Au moment de la création, le Pod est à l'état 'Pending' (téléchargement de l'image).
	// On doit scruter l'API K8s pendant quelques secondes jusqu'à ce qu'il soit 'Running'.
	t.Logf(" [WAIT] Attente que le Pod '%s' passe à l'état Running (Timeout: 30s)...", podName)

	podIsRunning := false
	maxAttempts := 30 // 30 tentatives
	pollInterval := 1 * time.Second // Attente d'1 seconde entre chaque tentative

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Récupération de l'état actuel du Pod (kubectl get pod nginx-test -n test-auto)
		currentPod, err := clientset.CoreV1().Pods(namespaceName).Get(ctx, podName, metav1.GetOptions{})
		
		if err == nil {
			// Phase représente le cycle de vie global : Pending, Running, Succeeded, Failed, Unknown
			t.Logf("   -> Tentative %d/%d : Statut actuel = %s", attempt, maxAttempts, currentPod.Status.Phase)

			if currentPod.Status.Phase == corev1.PodRunning {
				podIsRunning = true
				break // Succès ! On sort de la boucle d'attente
			}
		}

		time.Sleep(pollInterval)
	}

	// =========================================================================
	// 6. ASSERTION FINALE DU TEST
	// =========================================================================
	if !podIsRunning {
		t.Fatalf(" [ÉCHEC GLOBAL] Le Pod '%s' n'est pas passé en état Running dans le délai de 30s !", podName)
	}

	t.Logf(" [SUCCÈS VALIDATION] Le Pod '%s' dans le namespace '%s' est bien RUNNING !", podName, namespaceName)
}
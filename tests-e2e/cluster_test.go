package main

import (
	"context"   // Permet de gérer les délais d'expiration (timeouts) et l'annulation des requêtes
	"path/filepath" // Pour manipuler les chemins de fichiers de manière propre quel que soit l'OS
	"testing"   // Le framework de test natif de Go
	"time"      // Pour définir des durées (secondes, millisecondes)

	// Packages officiels Kubernetes (client-go)
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1" // Contient les types d'onglets metadata (ListOptions, GetOptions, etc.)
	"k8s.io/client-go/kubernetes"                 // Fournit le "clientset" pour interagir avec l'API K8s
	"k8s.io/client-go/tools/clientcmd"           // Utilitaires pour charger et lire le fichier kubeconfig
	"k8s.io/client-go/util/homedir"              // Permet de trouver le dossier personnel de l'utilisateur (~/)

	// corev1 : Contient les définitions des objets natifs K8s (Pods, Nodes, Services, Conditions, etc.).
	// On l'alias souvent "corev1" pour éviter toute confusion avec d'autres versions d'API.
	corev1 "k8s.io/api/core/v1"

)

// En Go, toute fonction de test DOIT commencer par "Test" et prendre "*testing.T" en paramètre.
func TestClusterAccessibility(t *testing.T) {
	
	// 1. & 2. Chargement simplifié du kubeconfig (utilise le contexte actif par défaut)
	home := homedir.HomeDir()
	if home == "" {
		t.Fatalf("Impossible de trouver le répertoire personnel ($HOME) de l'utilisateur.")
	}

	kubeconfigPath := filepath.Join(home, ".kube", "config")

	// La chaîne vide "" en premier argument indique à Go d'utiliser le contexte actif (default)
	clientConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		t.Fatalf("Échec du chargement du kubeconfig : %v", err)
	}

	// =========================================================================
	// 3. CRÉATION DU CLIENT KUBERNETES (CLIENTSET)
	// =========================================================================
	// Instancie le "clientset" officiel. C'est l'objet principal qui met à disposition
	// toutes les APIs Kubernetes (CoreV1, AppsV1, BatchV1, etc.).
	clientset, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		t.Fatalf("Impossible de créer le client Kubernetes : %v", err)
	}

	// =========================================================================
	// 4. GESTION DU TIMEOUT (CONTEXT)
	// =========================================================================
	// Les réseaux et clusters peuvent ne pas répondre. Pour éviter que le test ne bloque
	// indéfiniment, on crée un "Context" Go qui annulera automatiquement la requête au bout de 5s.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	
	// "defer" garantit que la fonction cancel() sera appelée à la fin de la fonction actuelle
	// afin de libérer les ressources mémoire immédiatement.
	defer cancel()

	// =========================================================================
	// 5. TEST DE CONNECTIVITÉ ET INTERROGATION DE L'API SERVER
	// =========================================================================
	t.Logf("Envoi d'une requête au cluster default")

	// On demande la liste des nœuds du cluster via le groupe d'API CoreV1.
	// C'est l'équivalent Go de la commande : kubectl get nodes
	// metav1.ListOptions{} indique qu'on ne filtre pas les résultats.
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	
	// Si une erreur survient (réseau coupé, mauvais port, droits insuffisants...), le test échoue.
	if err != nil {
		t.Fatalf(" Le cluster default n'est PAS accessible ! Erreur rencontrée : %v", err)
	}

	// =========================================================================
	// 6. ASSERTIONS ET VALIDATION DU RÉSULTAT
	// =========================================================================
	// Si la liste renvoyée est vide, le cluster n'a pas de worker/control-plane valide.
	if len(nodes.Items) == 0 {
		t.Fatalf("Connexion établie, mais AUCUN nœud n'a été trouvé dans le cluster default.")
	}

	// t.Logf affiche un message d'information de réussite (visible avec l'option -v lors du 'go test')
	t.Logf(" SUCCÈS : Le cluster default est joignable !")
	t.Logf("Nombre de nœud(s) détecté(s) : %d", len(nodes.Items))

	// Parcours et affichage du nom de chaque nœud récupéré
	for i, node := range nodes.Items {
		t.Logf(" - Nœud #%d : %s", i+1, node.Name)
	}
}

// =============================================================================
// FONCTION DE TEST E2E : TestClusterIsReady
// =============================================================================
// RÈGLES GO TESTING :
// 1. Le nom de la fonction DOIT impérativement commencer par "Test".
// 2. Le fichier DOIT se terminer par "_test.go".
// 3. Le seul argument autorisé est "t *testing.T" (pointeur vers la structure de gestion de test).
// =============================================================================
func TestClusterIsReady(t *testing.T) {

	// -------------------------------------------------------------------------
	// ÉTAPE 1 : RÉCUPÉRATION ET VALIDATION DU REPERTOIRE $HOME
	// -------------------------------------------------------------------------
	// En environnement WSL/Linux, cela renvoie typiquement : "/home/ton-utilisateur"
	home := homedir.HomeDir()
	if home == "" {
		// t.Fatalf : Affiche le message d'erreur ET interrompt immédiatement l'exécution du test.
		// Utilisé ici car sans dossier racine, impossible de localiser le kubeconfig.
		t.Fatalf(" [ERREUR CRITIQUE] Impossible de déterminer le répertoire $HOME de l'utilisateur.")
	}

	// -------------------------------------------------------------------------
	// ÉTAPE 2 : CHARGEMENT DU FICHIER DE CONFIGURATION KUBERNETES (KUBECONFIG)
	// -------------------------------------------------------------------------
	// Construction du chemin absolu : /home/utilisateur/.kube/config
	kubeconfigPath := filepath.Join(home, ".kube", "config")
	t.Logf(" Configuration : Lecture du fichier kubeconfig -> %s", kubeconfigPath)

	// clientcmd.BuildConfigFromFlags :
	// - Argument 1 ("") : Nom du master URL (laisser vide pour utiliser celui défini dans le kubeconfig).
	// - Argument 2 (kubeconfigPath) : Chemin vers le fichier kubeconfig.
	// Si le premier paramètre est vide, client-go utilise automatiquement le CONTEXTE ACTIF défini dans le fichier.
	clientConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		// %v est le verbe de formatage Go pour afficher la valeur par défaut d'une erreur.
		t.Fatalf(" [ÉCHEC] Impossible de charger le fichier kubeconfig : %v", err)
	}

	// -------------------------------------------------------------------------
	// ÉTAPE 3 : INITIALISATION DU CLIENT KUBERNETES (CLIENTSET)
	// -------------------------------------------------------------------------
	// Le Clientset instancie tous les clients d'API K8s (CoreV1, AppsV1, BatchV1, etc.).
	// C'est l'équivalent programmatique de la commande "kubectl".
	clientset, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		t.Fatalf(" [ÉCHEC] Impossible d'instancier le clientset Kubernetes : %v", err)
	}

	// -------------------------------------------------------------------------
	// ÉTAPE 4 : GESTION DU TIMEOUT ET DU CONTEXTE (CONTEXT)
	// -------------------------------------------------------------------------
	// Les appels réseau vers un cluster K8s (local ou distant) peuvent bloquer ou pendre indéfiniment.
	// On crée un context.WithTimeout qui annulera automatiquement toutes les requêtes associées
	// si le cluster ne répond pas dans un délai strict de 10 secondes.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	
	// "defer" est un mot-clé Go très puissant : il garantit que la fonction cancel() sera
	// TOUJOURS exécutée à la sortie de TestClusterIsReady, que le test réussisse ou échoue.
	// Cela permet de nettoyer immédiatement la mémoire et d'éviter les fuites de ressources (goroutine leaks).
	defer cancel()

	// -------------------------------------------------------------------------
	// ÉTAPE 5 : INTERROGATION DE L'API SERVER (RÉCUPÉRATION DES NŒUDS)
	// -------------------------------------------------------------------------
	t.Log(" Appel API Server : Récupération des nœuds du cluster...")

	// clientset.CoreV1().Nodes().List() est l'équivalent strict de la commande CLI : "kubectl get nodes"
	// - ctx : Transmet le timeout de 10s défini plus haut.
	// - metav1.ListOptions{} : Structure vide signifiant "aucun filtre/label selector appliqué".
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf(" [ÉCHEC] Erreur lors de la communication avec l'API Server K8s : %v", err)
	}

	// Assertion de sécurité : Un cluster fonctionnel doit posséder au moins 1 nœud (Control-Plane / Worker).
	if len(nodes.Items) == 0 {
		t.Fatalf(" [ÉCHEC VALIDATION] L'API Server répond, mais AUCUN nœud n'a été trouvé dans le cluster.")
	}

	// -------------------------------------------------------------------------
	// ÉTAPE 6 : INSPECTION DÉTAILLÉE DU STATUT 'READY' DE CHAQUE NŒUD
	// -------------------------------------------------------------------------
	// Compteur pour comptabiliser les nœuds défaillants.
	notReadyNodesCount := 0

	// Boucle "for range" : En Go, la structure "for index, element := range slice" permet d'itérer.
	// '_' indique qu'on ignore l'index du tableau.
	for _, node := range nodes.Items {
		isNodeReady := false

		// Dans Kubernetes, un Nœud possède un tableau de "Conditions" (MemoryPressure, DiskPressure, PIDPressure, Ready).
		// La condition "Ready" indique si le Kubelet sur le nœud est sain et prêt à accepter des Pods.
		for _, condition := range node.Status.Conditions {
			
			// On cherche spécifiquement la condition de type NodeReady ("Ready")
			if condition.Type == corev1.NodeReady {
				
				// Une condition K8s peut être True, False, ou Unknown.
				if condition.Status == corev1.ConditionTrue {
					isNodeReady = true
				} else {
					// Si le statut n'est pas True, on extrait la raison et le message explicatif transmis par K8s.
					t.Logf(" ⚠️ Nœud '%s' NON PRÊT ! Status=%s, Raison=%s, Message=%s", 
						node.Name, condition.Status, condition.Reason, condition.Message)
				}
				// Une fois la condition "Ready" trouvée, inutile de parcourir le reste des conditions de ce nœud.
				break
			}
		}

		// Validation du résultat pour le nœud courant
		if isNodeReady {
			t.Logf(" [OK] Nœud '%s' est à l'état READY", node.Name)
		} else {
			notReadyNodesCount++
		}
	}

	// -------------------------------------------------------------------------
	// ÉTAPE 7 : VERDICT FINAL DU TEST (ASSERTION GLOBALE)
	// -------------------------------------------------------------------------
	// Si au moins un nœud n'est pas prêt, le test est considéré comme ÉCHOUÉ.
	if notReadyNodesCount > 0 {
		t.Fatalf(" [ÉCHEC GLOBAL] %d nœud(s) sur %d ne sont pas Ready !", 
			notReadyNodesCount, len(nodes.Items))
	}

	// Message de confirmation si toutes les assertions ont réussi.
	t.Logf(" [SUCCÈS PARFAIT] Le cluster est 100%% fonctionnel et Ready (%d/%d nœud(s) prêt(s)) !", 
		len(nodes.Items), len(nodes.Items))
}
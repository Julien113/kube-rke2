# kube-rke2

Tests Automatisés E2E pour un Cluster Kubernetes avec Go

Objectif : Écrire des tests E2E en Go pour vérifier la santé et le bon fonctionnement d’un
cluster Kubernetes.

**Contexte**
Tu disposes d’un cluster RKE2. 
Tupeux créer un Namespace, par exemple test-auto, pour isoler cet exercice.
L’objectif est d’automatiserles tests suivants :
• L’accessibilité du cluster et l’état du nœud.
• L’état du Pod (Running, Pending, Failed...).
• La configuration et le bon fonctionnement des probes Liveness et Readiness.
• Le redémarrage automatique du Pod en cas d’échec de la Liveness Probe.
• La suppression du Pod après test.

Tâches à réaliser
1- Vérification de l’état du cluster
• Vérifier si l’API Kubernetes est accessible.
• Vérifier si le nœud du cluster est en état Ready.
2- Vérification de l’état du Pod
• Lister les Pods dans le namespace test- auto.
• Vérifier que le Pod nginx est bien en état Running.
3- Test des HealthChecks du Pod
• Vérifier que le Pod nginx a bien des probes Liveness et Readiness.
• Vérifier que la Readiness Probe passe (Pod Ready).
• Vérifier que la Liveness Probe fonctionne.
4- Simuler une panne de Liveness Probe
• Modifier la configuration du Pod pour forcer l’échec de la Liveness Probe.
• Vérifier que Kubernetes redémarre automatiquement le Pod.
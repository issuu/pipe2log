#!groovy

//
env.JENKINS_BUILD_NUMBER = env.BUILD_NUMBER

void colorize(Closure steps) {
    wrap($class: 'AnsiColorBuildWrapper', colorMapName: 'xterm') {
        steps()
    }
}

node {
    timestamps {
        colorize {
            stage(name: 'Checkout') {
                git(url: 'git@github.com:issuu/pipe2log.git', branch: env.BRANCH_NAME)
            }

            stage(name: 'DoIt') {
                // we should try and detect which project changed and only build that
                sh(script: 'make release')
            }

            stage(name: 'GithubRelease') {
                sh(script: 'make github-release')
            }

            stage(name: 'Archive') {
                archiveArtifacts(artifacts: '_rel/*')
            }

            // cleanup workspace ? do we need to ?
            stage(name: 'Cleanup Workspace') {
                step([$class: 'WsCleanup', cleanWhenAborted: false, cleanWhenFailure: false, cleanWhenNotBuilt: false, cleanWhenUnstable: false, notFailBuild: true])
            }
        }
    }
}

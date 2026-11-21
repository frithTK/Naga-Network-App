#include <jni.h>
#include <pthread.h>
#include <stdlib.h>

extern char *NagaStart(char *, char *);
extern void NagaStop(void);
extern void NagaStopRuntime(void);
extern char *NagaSetTunFd(int);
extern void NagaCloseTun(void);

static pthread_mutex_t protect_lock = PTHREAD_MUTEX_INITIALIZER;
static JavaVM *protect_vm;
static jclass protect_class;
static jmethodID protect_method;

static void init_protect(JNIEnv *env, jclass cls)
{
	pthread_mutex_lock(&protect_lock);
	if (protect_vm == NULL) {
		JavaVM *vm = NULL;
		jmethodID method = (*env)->GetStaticMethodID(env, cls, "protectAwgSocket", "(I)Z");
		if (method != NULL && (*env)->GetJavaVM(env, &vm) == JNI_OK) {
			jclass global = (*env)->NewGlobalRef(env, cls);
			if (global != NULL) {
				protect_class = global;
				protect_method = method;
				protect_vm = vm;
			}
		}
		if ((*env)->ExceptionCheck(env)) {
			(*env)->ExceptionClear(env);
		}
	}
	pthread_mutex_unlock(&protect_lock);
}

int naga_protect_awg_socket(int fd)
{
	pthread_mutex_lock(&protect_lock);
	JavaVM *vm = protect_vm;
	jclass cls = protect_class;
	jmethodID method = protect_method;
	pthread_mutex_unlock(&protect_lock);
	if (vm == NULL) {
		return 0;
	}
	JNIEnv *env = NULL;
	jint status = (*vm)->GetEnv(vm, (void **)&env, JNI_VERSION_1_6);
	int attached = 0;
	if (status == JNI_EDETACHED) {
		if ((*vm)->AttachCurrentThread(vm, &env, NULL) != JNI_OK) {
			return 0;
		}
		attached = 1;
	} else if (status != JNI_OK) {
		return 0;
	}
	jboolean result = (*env)->CallStaticBooleanMethod(env, cls, method, (jint)fd);
	if ((*env)->ExceptionCheck(env)) {
		(*env)->ExceptionClear(env);
		result = JNI_FALSE;
	}
	if (attached) {
		(*vm)->DetachCurrentThread(vm);
	}
	return result == JNI_TRUE;
}

static jstring goErr(JNIEnv *env, char *err)
{
	if (err == NULL) {
		return (*env)->NewStringUTF(env, "");
	}
	jstring out = (*env)->NewStringUTF(env, err);
	free(err);
	return out;
}

JNIEXPORT jstring JNICALL
Java_eu_nagavpn_naga_1network_NagaControl_jniStart(JNIEnv *env, jclass cls, jstring dir, jstring nativeDir)
{
	init_protect(env, cls);
	const char *data = (*env)->GetStringUTFChars(env, dir, 0);
	const char *native = (*env)->GetStringUTFChars(env, nativeDir, 0);
	char *err = NagaStart((char *)data, (char *)native);
	(*env)->ReleaseStringUTFChars(env, dir, data);
	(*env)->ReleaseStringUTFChars(env, nativeDir, native);
	return goErr(env, err);
}

JNIEXPORT void JNICALL
Java_eu_nagavpn_naga_1network_NagaControl_jniStop(JNIEnv *env, jclass cls)
{
	NagaStop();
}

JNIEXPORT void JNICALL
Java_eu_nagavpn_naga_1network_NagaControl_jniStopRuntime(JNIEnv *env, jclass cls)
{
	NagaStopRuntime();
}

JNIEXPORT jstring JNICALL
Java_eu_nagavpn_naga_1network_NagaControl_jniSetTunFd(JNIEnv *env, jclass cls, jint fd)
{
	return goErr(env, NagaSetTunFd((int)fd));
}

JNIEXPORT void JNICALL
Java_eu_nagavpn_naga_1network_NagaControl_jniCloseTun(JNIEnv *env, jclass cls)
{
	NagaCloseTun();
}

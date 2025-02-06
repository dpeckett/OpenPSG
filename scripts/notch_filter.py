import numpy as np
import matplotlib.pyplot as plt
from scipy.signal import iirnotch, freqz

# Parameters
fs = 40.0  # Sample frequency (Hz)
f0 = 4.5   # Notch frequency (Hz)
Q = 0.5    # Quality factor - controls the width of the notch

# Generate the IIR notch filter coefficients
b, a = iirnotch(w0=f0, Q=Q, fs=fs)

# Frequency response
freq, response = freqz(b, a, worN=8000, fs=fs)

# Plot the frequency response
plt.figure(figsize=(10, 5))
plt.plot(freq, 20 * np.log10(np.abs(response)), 'b')
plt.title('Frequency Response of the IIR Notch Filter')
plt.xlabel('Frequency [Hz]')
plt.ylabel('Amplitude [dB]')
plt.grid()
plt.show()

print("IIR Notch Filter Coefficients:")
print("b (numerator):", b)
print("a (denominator):", a)

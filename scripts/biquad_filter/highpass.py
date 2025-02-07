import numpy as np
import matplotlib.pyplot as plt
from scipy.signal import butter, freqz

# Parameters
fs = 40.0  # Sample frequency (Hz)
fc = 0.1   # Cut-off frequency (Hz)

# Generate the Butterworth high-pass filter coefficients
b, a = butter(N=2, Wn=fc/(fs/2), btype='high', analog=False)

# Frequency response
freq, response = freqz(b, a, worN=8000, fs=fs)

# Plot the frequency response
plt.figure(figsize=(10, 5))
plt.plot(freq, 20 * np.log10(np.abs(response)), 'b')
plt.title('Frequency Response of the High-Pass Filter')
plt.xlabel('Frequency [Hz]')
plt.ylabel('Amplitude [dB]')
plt.grid()
plt.show()

print("High-Pass Filter Coefficients:")
print("b (numerator):", b)
print("a (denominator):", a)
